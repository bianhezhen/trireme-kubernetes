// Package resolver resolves each Container to a specific Trireme policy
// based on Kubernetes Policy definitions.
package resolver

import (
	"encoding/json"
	"fmt"

	"github.com/aporeto-inc/trireme-kubernetes/kubernetes"

	"github.com/aporeto-inc/kubepox"
	trireme "github.com/aporeto-inc/trireme-lib"
	"github.com/aporeto-inc/trireme-lib/monitor"
	"github.com/aporeto-inc/trireme-lib/policy"

	api "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/labels"

	"go.uber.org/zap"
)

// KubernetesPolicy represents a Trireme Policer for Kubernetes.
// It implements the Trireme Resolver interface and implements the policies defined
// by Kubernetes NetworkPolicy API.
type KubernetesPolicy struct {
	triremeNetworks  []string
	policyUpdater    trireme.PolicyUpdater
	KubernetesClient *kubernetes.Client
	betaPolicies     bool
	egressPolicies   bool
	cache            *cache
	stopAll          chan struct{}
}

// NewKubernetesPolicy creates a new policy engine for the Trireme package
func NewKubernetesPolicy(kubeconfig string, nodename string, triremeNetworks []string, betaPolicies bool, egressPolicies bool) (*KubernetesPolicy, error) {
	client, err := kubernetes.NewClient(kubeconfig, nodename)
	if err != nil {
		return nil, fmt.Errorf("Couldn't create KubernetesClient: %v ", err)
	}

	return &KubernetesPolicy{
		triremeNetworks:  triremeNetworks,
		KubernetesClient: client,
		betaPolicies:     betaPolicies,
		egressPolicies:   egressPolicies,
		cache:            newCache(),
	}, nil
}

// isNamespaceNetworkPolicyActive returns true if the namespace has NetworkPolicies
// activated on the annotation
func isNamespaceNetworkPolicyActive(namespace *api.Namespace) bool {
	// Statically never actvating anything into Kube-System namespace.
	// TODO: Allow KubeSystem to have networking policies enabled ?
	if namespace.GetName() == "kube-system" {
		return false
	}

	// Check if annotation is present. As NetworkPolicies in K8s are still beta
	// The format needs to be manually parsed out of JSON.
	value, ok := namespace.GetAnnotations()[KubernetesNetworkPolicyAnnotationID]

	if !ok {
		return false
	}
	networkPolicyAnnotation := &NamespaceNetworkPolicy{}
	if err := json.Unmarshal([]byte(value), networkPolicyAnnotation); err != nil {
		return false
	}

	if networkPolicyAnnotation != nil &&
		networkPolicyAnnotation.Ingress != nil &&
		networkPolicyAnnotation.Ingress.Isolation != nil &&
		*networkPolicyAnnotation.Ingress.Isolation == DefaultDeny {
		return true
	}
	return false
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

// SetPolicyUpdater registers the interface used for updating Policies explicitely.
func (k *KubernetesPolicy) SetPolicyUpdater(policyUpdater trireme.PolicyUpdater) error {
	k.policyUpdater = policyUpdater
	return nil
}

// ResolvePolicy generates the Policy for the target PU.
// The policy for the PU will be based on the defined
// Kubernetes NetworkPolicies on the Pod to which the PU belongs.
func (k *KubernetesPolicy) ResolvePolicy(contextID string, runtimeGetter policy.RuntimeReader) (*policy.PUPolicy, error) {

	// Only the Infra Container should be policed. All the others should be AllowAll.
	// The Infra container can be found by checking env. variable.
	tagContent, ok := runtimeGetter.Tag(KubernetesContainerName)
	if !ok || tagContent != KubernetesInfraContainerName {
		// return AllowAll
		zap.L().Info("Container is not Infra Container. AllowingAll", zap.String("contextID", contextID))
		return notInfraContainerPolicy(), nil
	}

	podName, ok := runtimeGetter.Tag(KubernetesPodName)
	if !ok {
		return nil, fmt.Errorf("Error getting Kubernetes Pod name")
	}
	podNamespace, ok := runtimeGetter.Tag(KubernetesPodNamespace)
	if !ok {
		return nil, fmt.Errorf("Error getting Kubernetes Pod namespace")
	}

	// Keep the mapping in cache: ContextID <--> PodNamespace/PodName
	k.cache.addPodToCache(contextID, podName, podNamespace)
	return k.resolvePodPolicy(podName, podNamespace)
}

// HandlePUEvent  is called by Trireme for notification that a specific PU got an event.
func (k *KubernetesPolicy) HandlePUEvent(contextID string, eventType monitor.Event) {
	zap.L().Debug("Trireme Container Event", zap.String("contextID", contextID), zap.Any("eventType", eventType))
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

	// Generating all the rules and generate policy.

	// TODO: Quick hack to generate NetworkPolicy from the store instead than from the API.
	// Replace this with correct API Calls whenever client-go will support it.
	nsWatcher, exist := k.cache.getNamespaceWatcher(kubernetesNamespace)
	if !exist {
		return nil, fmt.Errorf("Couldn't find active Namespace %s ", kubernetesNamespace)
	}

	namespaceRules, err := nsWatcher.getPolicyList()
	if err != nil {
		return nil, fmt.Errorf("Couldn't generate current NetPolicies for the namespace %s ", kubernetesNamespace)
	}

	ingressPodRules, err := k.KubernetesClient.IngressPodRules(kubernetesPod, kubernetesNamespace, namespaceRules)
	if err != nil {
		return nil, fmt.Errorf("Couldn't get the NetworkPolicies for Pod %s : %s", kubernetesPod, err)
	}

	egressPodRules, err := k.KubernetesClient.EgressPodRules(kubernetesPod, kubernetesNamespace, namespaceRules)
	if err != nil {
		return nil, fmt.Errorf("Couldn't get the NetworkPolicies for Pod %s : %s", kubernetesPod, err)
	}

	allNamespaces, _ := k.KubernetesClient.AllNamespaces()

	ips := policy.ExtendedMap{policy.DefaultNamespace: pod.Status.PodIP}

	puPolicy, err := generatePUPolicy(ingressPodRules, egressPodRules, kubernetesNamespace, allNamespaces, policy.NewTagStoreFromMap(podLabels), ips, k.triremeNetworks, k.betaPolicies, k.egressPolicies)
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

	if k.policyUpdater == nil {
		return fmt.Errorf("PolicyUpdate failed: No PolicyUpdater registered")
	}

	// Finding back the ContextID for that specificPod.
	contextID, err := k.cache.contextIDByPodName(podName, podNamespace)
	if err != nil {
		return fmt.Errorf("Error finding pod in cache for update: %s", err)
	}

	// Regenerating a Full Policy and Tags.
	containerPolicy, err := k.resolvePodPolicy(podName, podNamespace)
	if err != nil {
		return fmt.Errorf("Couldn't generate a Pod Policy for pod update %s", err)
	}
	err = k.policyUpdater.UpdatePolicy(contextID, containerPolicy)
	if err != nil {
		return fmt.Errorf("Error while updating the policy: %s", err)
	}
	return nil
}

// activateNamespace starts to watch the pods and networkpolicies in the parameter namespace.
func (k *KubernetesPolicy) activateNamespace(namespace *api.Namespace) error {
	zap.L().Info("Activating namespace for NetworkPolicies", zap.String("namespace", namespace.GetName()))

	podControllerStop := make(chan struct{})
	podStore, podController := k.KubernetesClient.CreateLocalPodController(namespace.GetName(),
		k.addPod,
		k.deletePod,
		k.updatePod)
	go podController.Run(podControllerStop)
	zap.L().Debug("Pod Controller created", zap.String("namespace", namespace.GetName()))

	npControllerStop := make(chan struct{})
	npStore, npController := k.KubernetesClient.CreateNetworkPoliciesController(namespace.Name,
		k.addNetworkPolicy,
		k.deleteNetworkPolicy,
		k.updateNetworkPolicy)
	go npController.Run(npControllerStop)
	zap.L().Debug("NetworkPolicy controller created", zap.String("namespace", namespace.GetName()))

	namespaceWatcher := NewNamespaceWatcher(namespace.Name, podStore, podController, podControllerStop, npStore, npController, npControllerStop)
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
func (k *KubernetesPolicy) Run() {
	k.stopAll = make(chan struct{})
	_, nsController := k.KubernetesClient.CreateNamespaceController(
		k.addNamespace,
		k.deleteNamespace,
		k.updateNamespace)
	go nsController.Run(k.stopAll)
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

	if !k.betaPolicies {
		// Every namespace is activated under GA networkpolicies
		zap.L().Info("Namespace Added. Activating GA NetworkPolicies", zap.String("namespace", addedNS.GetName()))
		return k.activateNamespace(addedNS)
	}

	if !isNamespaceNetworkPolicyActive(addedNS) {
		// Beta Policies: Namespace doesn't have Beta NetworkPolicies annotations
		zap.L().Info("Namespace Added. Doesn't have Beta NetworkPolicies annotations. Not activating", zap.String("namespace", addedNS.GetName()))
		return nil
	}

	// Beta Policies: Namespace has the annotations
	zap.L().Info("Namespace Added. Has Beta NetworkPolicies. Activating", zap.String("namespace", addedNS.GetName()))
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
	if !k.betaPolicies {
		// GA Policies. No changes.
		return nil
	}

	// Beta NetworkPolicies: Checking if Namespace needs to be activated//deactivated.
	if isNamespaceNetworkPolicyActive(updatedNS) {
		if k.cache.isNamespaceActive(updatedNS.GetName()) {
			zap.L().Info("Namespace Modified. Already active for beta NetPolicies", zap.String("namespace", updatedNS.GetName()))
			return nil
		}
		zap.L().Info("Namespace Modified. Activating for beta NetPolicies", zap.String("namespace", updatedNS.GetName()))
		return k.activateNamespace(updatedNS)
	}

	if k.cache.isNamespaceActive(updatedNS.GetName()) {
		zap.L().Info("Namespace Modified. Deactivating for beta NetPolicies", zap.String("namespace", updatedNS.GetName()))
		return k.deactivateNamespace(updatedNS)
	}
	zap.L().Info("Namespace Modified. Doesn't have Beta NetworkPolicies annotations. Not activating", zap.String("namespace", updatedNS.GetName()))
	return nil
}

func (k *KubernetesPolicy) addPod(addedPod *api.Pod) error {
	zap.L().Debug("Pod Added", zap.String("name", addedPod.GetName()), zap.String("namespace", addedPod.GetNamespace()))

	err := k.updatePodPolicy(addedPod)
	if err != nil {
		return fmt.Errorf("Failed UpdatePolicy on NewPodEvent: %s", err)
	}
	return nil
}

func (k *KubernetesPolicy) deletePod(deletedPod *api.Pod) error {
	zap.L().Debug("Pod Deleted", zap.String("name", deletedPod.GetName()), zap.String("namespace", deletedPod.GetNamespace()))

	err := k.cache.deleteFromCacheByPodName(deletedPod.GetName(), deletedPod.GetNamespace())
	if err != nil {
		return fmt.Errorf("Error for PodDelete: %s ", err)
	}
	return nil
}

func (k *KubernetesPolicy) updatePod(oldPod, updatedPod *api.Pod) error {
	zap.L().Debug("Pod Modified detected", zap.String("name", updatedPod.GetName()), zap.String("namespace", updatedPod.GetNamespace()))

	if !isPolicyUpdateNeeded(oldPod, updatedPod) {
		zap.L().Debug("No modified labels for Pod", zap.String("name", updatedPod.GetName()), zap.String("namespace", updatedPod.GetNamespace()))
		return nil
	}
	err := k.updatePodPolicy(updatedPod)
	if err != nil {
		return fmt.Errorf("Failed UpdatePolicy on ModifiedPodEvent. Probably related to ongoing delete: %s", err)
	}
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
