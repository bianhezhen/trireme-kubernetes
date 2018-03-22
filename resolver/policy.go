// Package resolver resolves each Container to a specific Trireme policy
// based on Kubernetes Policy definitions.
package resolver

import (
	"context"
	"fmt"
	"time"

	"github.com/aporeto-inc/trireme-kubernetes/kubernetes"

	"github.com/aporeto-inc/kubepox"
	"github.com/aporeto-inc/trireme-lib/common"
	"github.com/aporeto-inc/trireme-lib/controller"
	"github.com/aporeto-inc/trireme-lib/policy"

	api "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"

	"go.uber.org/zap"
)

// KubernetesPolicy represents a Trireme Policer for Kubernetes.
// It implements the Trireme Resolver interface and implements the policies defined
// by Kubernetes NetworkPolicy API.
type KubernetesPolicy struct {
	controller       controller.TriremeController
	triremeNetworks  []string
	KubernetesClient *kubernetes.Client
	cache            *cacheStruct
	stopAll          chan struct{}
}

// NewKubernetesPolicy creates a new policy engine for the Trireme package
func NewKubernetesPolicy(ctx context.Context, controller controller.TriremeController, kubeconfig string, nodename string, triremeNetworks []string) (*KubernetesPolicy, error) {
	client, err := kubernetes.NewClient(kubeconfig, nodename)
	if err != nil {
		return nil, fmt.Errorf("Couldn't create KubernetesClient: %v ", err)
	}

	return &KubernetesPolicy{
		controller:       controller,
		triremeNetworks:  triremeNetworks,
		KubernetesClient: client,
		cache:            newCache(),
	}, nil
}

// isNamespaceKubeSystem returns true if the namespace is kube-system
func isNamespaceKubeSystem(namespace string) bool {
	return namespace == "kube-system"
}

func isPolicyUpdateNeeded(oldPod, newPod *api.Pod) bool {
	if !(oldPod.Status.PodIP == newPod.Status.PodIP) {
		return true
	}
	if !labels.Equals(oldPod.GetLabels(), newPod.GetLabels()) {
		return true
	}
	return false
}

// ResolvePolicy generates the Policy for the target PU.
// The policy for the PU will be based on the defined
// Kubernetes NetworkPolicies on the Pod to which the PU belongs.
func (k *KubernetesPolicy) ResolvePolicy(contextID string, runtime policy.RuntimeReader) (*policy.PUPolicy, error) {

	// Only the Infra Container should be policed. All the others should be AllowAll.
	// The Infra container can be found by checking env. variable.
	tagContent, ok := runtime.Tag(KubernetesContainerName)
	if !ok || tagContent != KubernetesInfraContainerName {
		// return AllowAll
		zap.L().Info("Container is not Infra Container. AllowingAll", zap.String("contextID", contextID))
		return notInfraContainerPolicy(), nil
	}

	podName, ok := runtime.Tag(KubernetesPodName)
	if !ok {
		return nil, fmt.Errorf("Error getting Kubernetes Pod name")
	}
	podNamespace, ok := runtime.Tag(KubernetesPodNamespace)
	if !ok {
		return nil, fmt.Errorf("Error getting Kubernetes Pod namespace")
	}

	// Keep the mapping in cache: ContextID <--> PodNamespace/PodName
	k.cache.addPodToCache(contextID, runtime, podName, podNamespace)
	return k.resolvePodPolicy(podName, podNamespace)
}

// HandlePUEvent  is called by Trireme for notification that a specific PU got an event.
func (k *KubernetesPolicy) HandlePUEvent(ctx context.Context, puID string, event common.Event, runtime policy.RuntimeReader) error {
	zap.L().Debug("Trireme Container Event", zap.String("contextID", puID), zap.Any("eventType", event))

	// We only add on the start of a DockeeContainer. All the other events are directly comming from Kubernetes API.

	switch event {
	case common.EventStart:
		resolvedPolicy, err := k.ResolvePolicy(puID, runtime)
		if err != nil {
			return err
		}

		// TODO: Better management of PURuntime (no casting)
		err = k.controller.Enforce(ctx, puID, resolvedPolicy, runtime.(*policy.PURuntime))
		if err != nil {
			return fmt.Errorf("Error while creating the policy: %s", err)
		}

	case common.EventCreate:
	case common.EventDestroy:
	case common.EventPause:
	case common.EventUnpause:
	}

	return nil
}

// resolvePodPolicy generates the Trireme Policy for a specific Kube Pod and Namespace.
func (k *KubernetesPolicy) resolvePodPolicy(kubernetesPod string, kubernetesNamespace string) (*policy.PUPolicy, error) {
	// Query Kube API to get the Pod's label and IP.
	zap.L().Info("Resolving policy for POD", zap.String("name", kubernetesPod), zap.String("namespace", kubernetesNamespace))
	pod, err := k.KubernetesClient.Pod(kubernetesPod, kubernetesNamespace)
	if err != nil {
		return nil, fmt.Errorf("Couldn't get labels for pod %s : %v", kubernetesPod, err)
	}

	// If IP is empty, wait for an UpdatePodEvent with the Actual PodIP. Not ready to be activated now.
	if pod.Status.PodIP == "" {
		return notInfraContainerPolicy(), nil
	}
	// If Pod is running in the hostNS , no activation (not supported).
	if pod.Status.PodIP == pod.Status.HostIP {
		return notInfraContainerPolicy(), nil
	}

	podLabels := pod.GetLabels()
	if podLabels == nil {
		return notInfraContainerPolicy(), nil
	}

	// Check if the Pod's namespace is activated.
	if !k.cache.isNamespaceActive(kubernetesNamespace) {

		zap.L().Info("Pod namespace is not NetworkPolicyActivated, AllowAll", zap.String("podNamespace", kubernetesNamespace))
		// adding the namespace as an extra label.
		podLabels["@namespace"] = kubernetesNamespace
		ips := policy.ExtendedMap{policy.DefaultNamespace: pod.Status.PodIP}
		allowAllPuPolicy := allowAllPolicy(policy.NewTagStoreFromMap(podLabels), ips, k.triremeNetworks)

		return allowAllPuPolicy, nil
	}

	// adding the namespace as an extra label.
	podLabels["@namespace"] = kubernetesNamespace

	nsNetworkPolicies, err := k.KubernetesClient.NetworkPolicies(kubernetesNamespace)
	if err != nil {
		return nil, fmt.Errorf("Couldn't generate current NetPolicies for the namespace %s ", kubernetesNamespace)
	}

	ingressPodRules, err := k.KubernetesClient.IngressPodRules(kubernetesPod, kubernetesNamespace, nsNetworkPolicies)
	if err != nil {
		return nil, fmt.Errorf("Couldn't get the NetworkPolicies for Pod %s : %s", kubernetesPod, err)
	}

	egressPodRules, err := k.KubernetesClient.EgressPodRules(kubernetesPod, kubernetesNamespace, nsNetworkPolicies)
	if err != nil {
		return nil, fmt.Errorf("Couldn't get the NetworkPolicies for Pod %s : %s", kubernetesPod, err)
	}

	allNamespaces, _ := k.KubernetesClient.AllNamespaces()

	ips := policy.ExtendedMap{policy.DefaultNamespace: pod.Status.PodIP}

	puPolicy, err := generatePUPolicy(ingressPodRules, egressPodRules, kubernetesNamespace, allNamespaces, policy.NewTagStoreFromMap(podLabels), ips, k.triremeNetworks)
	if err != nil {
		return nil, err
	}

	return puPolicy, nil
}

// updatePodPolicy updates (and replace) the policy of the pod given in parameter.
func (k *KubernetesPolicy) updatePodPolicy(pod *api.Pod) error {
	podName := pod.GetName()
	podNamespace := pod.GetNamespace()
	zap.L().Info("Update pod Policy", zap.String("podNamespace", podNamespace), zap.String("podName", podName))

	if k.controller == nil {
		return fmt.Errorf("PolicyUpdate failed: No PolicyUpdater registered")
	}

	// Finding back the ContextID for that specificPod.
	contextID, err := k.cache.contextIDByPodName(podName, podNamespace)
	if err != nil {
		return fmt.Errorf("Error finding pod in cache for update: %s", err)
	}

	runtime, err := k.cache.runtimeByPodName(podName, podNamespace)
	if err != nil {
		return fmt.Errorf("Error finding pod in cache for update: %s", err)
	}

	// Regenerating a Full Policy and Tags.
	containerPolicy, err := k.resolvePodPolicy(podName, podNamespace)
	if err != nil {
		return fmt.Errorf("Couldn't generate a Pod Policy for pod update %s", err)
	}

	// TODO: Eventually find a way to not cast explicitely.
	err = k.controller.UpdatePolicy(context.TODO(), contextID, containerPolicy, runtime.(*policy.PURuntime))
	if err != nil {
		return fmt.Errorf("Error while updating the policy: %s", err)
	}

	return nil
}

// activateNamespace starts to watch the pods and networkpolicies in the parameter namespace.
func (k *KubernetesPolicy) activateNamespace(namespace *api.Namespace) error {
	zap.L().Info("Activating namespace for NetworkPolicies", zap.String("namespace", namespace.GetName()))

	npControllerStop := make(chan struct{})
	npStore, npController := k.KubernetesClient.CreateNetworkPoliciesController(namespace.Name,
		k.addNetworkPolicy,
		k.deleteNetworkPolicy,
		k.updateNetworkPolicy)
	go npController.Run(npControllerStop)
	zap.L().Debug("NetworkPolicy controller created", zap.String("namespace", namespace.GetName()))

	namespaceWatcher := NewNamespaceWatcher(namespace.Name, npStore, npController, npControllerStop)
	k.cache.activateNamespaceWatcher(namespace.GetName(), namespaceWatcher)
	zap.L().Debug("Finished namespace activation", zap.String("namespace", namespace.GetName()))

	return nil
}

// deactivateNamespace stops all the watching on the specified namespace.
func (k *KubernetesPolicy) deactivateNamespace(namespace *api.Namespace) error {
	zap.L().Info("Deactivating namespace for NetworkPolicies ", zap.String("namespace", namespace.GetName()))
	k.cache.deactivateNamespaceWatcher(namespace.GetName())
	return nil
}

// Run starts the KubernetesPolicer by watching for Namespace Changes.
// Run is blocking. Use go
func (k *KubernetesPolicy) Run(sync chan struct{}) {
	k.stopAll = make(chan struct{})
	_, nsController := k.KubernetesClient.CreateNamespaceController(
		k.addNamespace,
		k.deleteNamespace,
		k.updateNamespace)
	nsController.HasSynced()
	go nsController.Run(k.stopAll)

	if sync != nil {
		go hasSynced(sync, nsController)
	}
}

// Stop Stops all the channels
func (k *KubernetesPolicy) Stop() {
	k.stopAll <- struct{}{}
	for _, namespaceWatcher := range k.cache.namespaceActivation {
		namespaceWatcher.stopWatchingNamespace()
	}
}

func (k *KubernetesPolicy) addNamespace(addedNS *api.Namespace) error {
	if k.cache.isNamespaceActive(addedNS.GetName()) {
		// Namespace already activated
		zap.L().Info("Namespace Added. already active", zap.String("namespace", addedNS.GetName()))
		return nil
	}

	// Every namespace is activated under GA networkpolicies
	zap.L().Info("Namespace Added. Activating GA NetworkPolicies", zap.String("namespace", addedNS.GetName()))
	return k.activateNamespace(addedNS)
}

func (k *KubernetesPolicy) deleteNamespace(deletedNS *api.Namespace) error {
	if k.cache.isNamespaceActive(deletedNS.GetName()) {
		zap.L().Info("Namespace Deleted. Removing", zap.String("namespace", deletedNS.GetName()))
		return k.deactivateNamespace(deletedNS)
	}
	return nil
}

func (k *KubernetesPolicy) updateNamespace(oldNS, updatedNS *api.Namespace) error {
	// GA Policies. No changes.
	return nil

}

func (k *KubernetesPolicy) addNetworkPolicy(addedNP *networking.NetworkPolicy) error {
	zap.L().Debug("NetworkPolicy Added.", zap.String("name", addedNP.GetName()), zap.String("namespace", addedNP.GetNamespace()))

	// TODO: Filter on pods from localNode only.
	allLocalPods, err := k.KubernetesClient.LocalPods(addedNP.Namespace)
	if err != nil {
		return fmt.Errorf("Couldn't get all local pods: %s", err)
	}
	affectedPods, err := kubepox.ListPodsPerPolicy(addedNP, allLocalPods)
	if err != nil {
		return fmt.Errorf("Couldn't get all pods for policy: %s , %s ", addedNP.GetName(), err)
	}
	//Reresolve all affected pods
	for _, pod := range affectedPods.Items {
		zap.L().Debug("Updating pod based on a K8S NetworkPolicy Change", zap.String("name", pod.Name), zap.String("namespace", pod.Namespace))
		err := k.updatePodPolicy(&pod)
		if err != nil {
			return fmt.Errorf("UpdatePolicy failed: %s", err)
		}
	}
	return nil
}

func (k *KubernetesPolicy) deleteNetworkPolicy(deletedNP *networking.NetworkPolicy) error {
	zap.L().Debug("NetworkPolicy Deleted.", zap.String("name", deletedNP.GetName()), zap.String("namespace", deletedNP.GetNamespace()))

	// TODO: Filter on pods from localNode only.
	allLocalPods, err := k.KubernetesClient.LocalPods(deletedNP.Namespace)
	if err != nil {
		return fmt.Errorf("Couldn't get all local pods: %s", err)
	}
	affectedPods, err := kubepox.ListPodsPerPolicy(deletedNP, allLocalPods)
	if err != nil {
		return fmt.Errorf("Couldn't get all pods for policy: %s , %s ", deletedNP.GetName(), err)
	}
	//Reresolve all affected pods
	for _, pod := range affectedPods.Items {
		zap.L().Debug("Updating pod based on a K8S NetworkPolicy Change", zap.String("name", pod.GetName()), zap.String("namespace", pod.GetNamespace()))
		err := k.updatePodPolicy(&pod)
		if err != nil {
			return fmt.Errorf("UpdatePolicy failed: %s", err)
		}
	}
	return nil
}

func (k *KubernetesPolicy) updateNetworkPolicy(oldNP, updatedNP *networking.NetworkPolicy) error {
	zap.L().Debug("NetworkPolicy Modified", zap.String("name", updatedNP.GetName()), zap.String("namespace", updatedNP.GetNamespace()))

	// TODO: Filter on pods from localNode only.
	allLocalPods, err := k.KubernetesClient.LocalPods(updatedNP.Namespace)
	if err != nil {
		return fmt.Errorf("Couldn't get all local pods: %s", err)
	}
	affectedPods, err := kubepox.ListPodsPerPolicy(updatedNP, allLocalPods)
	if err != nil {
		return fmt.Errorf("Couldn't get all pods for policy: %s , %s ", updatedNP.GetName(), err)
	}
	//Reresolve all affected pods
	for _, pod := range affectedPods.Items {
		zap.L().Debug("Updating pod based on a K8S NetworkPolicy Change", zap.String("name", pod.GetName()), zap.String("name", pod.GetNamespace()))
		err := k.updatePodPolicy(&pod)
		if err != nil {
			return fmt.Errorf("UpdatePolicy failed: %s", err)
		}
	}
	return nil
}

// hasSynced sends an event on the Sync chan when the attachedController finished syncing.
func hasSynced(sync chan struct{}, controller cache.Controller) {
	for true {
		if controller.HasSynced() {
			sync <- struct{}{}
			return
		}
		<-time.After(100 * time.Millisecond)
	}
}
