package kubernetes

import (
	"fmt"

	"github.com/aporeto-inc/kubepox"

	api "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Client is the Trireme representation of the Client.
type Client struct {
	kubeClient kubernetes.Interface
	localNode  string
}

// NewClient Generate and initialize a Trireme Client object
func NewClient(kubeconfig string, nodename string) (*Client, error) {
	Client := &Client{}
	Client.localNode = nodename

	if err := Client.InitKubernetesClient(kubeconfig); err != nil {
		return nil, fmt.Errorf("Couldn't initialize Kubernetes Client: %v", err)
	}
	return Client, nil
}

// InitKubernetesClient Initialize the Kubernetes client based on the parameter kubeconfig
// if Kubeconfig is empty, try an in-cluster auth.
func (c *Client) InitKubernetesClient(kubeconfig string) error {

	var config *restclient.Config
	var err error

	if kubeconfig == "" {
		// TODO: Explicit InCluster config call.
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return fmt.Errorf("Error Building InCluster config: %v", err)
		}
	} else {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return fmt.Errorf("Error Building config from Kubeconfig: %v", err)
		}
	}

	myClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("Error creating REST Kube Client: %v", err)
	}
	c.kubeClient = myClient
	return nil
}

func (c *Client) localNodeSelector() fields.Selector {
	return fields.Set(map[string]string{
		"spec.nodeName": c.localNode,
	}).AsSelector()
}

func (c *Client) localNodeOption() metav1.ListOptions {
	return metav1.ListOptions{
		FieldSelector: c.localNodeSelector().String(),
	}
}

// IngressPodRules return the list of all the IngressRules that apply to the pod.
func (c *Client) IngressPodRules(podName string, namespace string, allPolicies *networking.NetworkPolicyList) (*[]networking.NetworkPolicyIngressRule, error) {
	// Step1: Get all the rules associated with this Pod.
	targetPod, err := c.kubeClient.Core().Pods(namespace).Get(podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("Couldn't get pod %v from Kubernetes API: %v", podName, err)
	}

	allRules, err := kubepox.ListIngressRulesPerPod(targetPod, allPolicies)
	if err != nil {
		return nil, fmt.Errorf("Couldn't process the list of rules for pod %v : %v", podName, err)
	}
	return allRules, nil
}

// EgressPodRules return the list of all the IngressRules that apply to the pod.
func (c *Client) EgressPodRules(podName string, namespace string, allPolicies *networking.NetworkPolicyList) (*[]networking.NetworkPolicyEgressRule, error) {
	// Step1: Get all the rules associated with this Pod.
	targetPod, err := c.kubeClient.Core().Pods(namespace).Get(podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("Couldn't get pod %v from Kubernetes API: %v", podName, err)
	}

	allRules, err := kubepox.ListEgressRulesPerPod(targetPod, allPolicies)
	if err != nil {
		return nil, fmt.Errorf("Couldn't process the list of rules for pod %v : %v", podName, err)
	}
	return allRules, nil
}

// Endpoints return the list of all the Endpoints that are serviced by a specific service/namespace.
func (c *Client) Endpoints(service string, namespace string) (*api.Endpoints, error) {
	// Step1: Get all the rules associated with this Pod.
	endpoints, err := c.kubeClient.Core().Endpoints(namespace).Get(service, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("Couldn't get endpoints for service %s from Kubernetes API: %s", service, err)
	}
	return endpoints, nil
}

// PodLabels returns the list of all labels associated with a pod.
func (c *Client) PodLabels(podName string, namespace string) (map[string]string, error) {
	targetPod, err := c.kubeClient.Core().Pods(namespace).Get(podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error getting Kubernetes labels for pod %v : %v ", podName, err)
	}
	return targetPod.GetLabels(), nil
}

// PodIP returns the pod's IP.
func (c *Client) PodIP(podName string, namespace string) (string, error) {
	targetPod, err := c.kubeClient.Core().Pods(namespace).Get(podName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("error getting Kubernetes IP for pod %v : %v ", podName, err)
	}
	return targetPod.Status.PodIP, nil
}

// PodLabelsAndIP returns the list of all labels associated with a pod as well as the Pod's IP.
func (c *Client) PodLabelsAndIP(podName string, namespace string) (map[string]string, string, error) {
	targetPod, err := c.kubeClient.Core().Pods(namespace).Get(podName, metav1.GetOptions{})
	if err != nil {
		return nil, "", fmt.Errorf("error getting Kubernetes labels & IP for pod %v : %v ", podName, err)
	}
	ip := targetPod.Status.PodIP
	if targetPod.Status.PodIP == targetPod.Status.HostIP {
		ip = "host"
	}
	return targetPod.GetLabels(), ip, nil
}

// Pod returns the full pod object.
func (c *Client) Pod(podName string, namespace string) (*api.Pod, error) {
	targetPod, err := c.kubeClient.Core().Pods(namespace).Get(podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error getting Kubernetes labels & IP for pod %v : %v ", podName, err)
	}
	return targetPod, nil
}

// LocalPods return a PodList with all the pods scheduled on the local node
func (c *Client) LocalPods(namespace string) (*api.PodList, error) {
	return c.kubeClient.Core().Pods(namespace).List(c.localNodeOption())
}

// AllNamespaces return a list of all existing namespaces
func (c *Client) AllNamespaces() (*api.NamespaceList, error) {
	return c.kubeClient.Core().Namespaces().List(metav1.ListOptions{})
}

// AddLocalNodeAnnotation adds the annotationKey:annotationValue
func (c *Client) AddLocalNodeAnnotation(annotationKey, annotationValue string) error {
	nodeName := c.localNode
	node, err := c.kubeClient.Core().Nodes().Get(nodeName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("Couldn't get node %s: %s", nodeName, err)
	}

	annotations := node.GetAnnotations()
	annotations[annotationKey] = annotationValue
	node.SetAnnotations(annotations)
	_, err = c.kubeClient.Core().Nodes().Update(node)
	if err != nil {
		return fmt.Errorf("Error updating Annotations for node %s: %s", nodeName, err)
	}
	return nil
}

// AllNodes return a list of all the nodes on the KubeCluster.
func (c *Client) AllNodes() (*api.NodeList, error) {
	nodes, err := c.kubeClient.Core().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("Couldn't get nodes list : %s", err)
	}
	return nodes, nil
}

// NetworkPolicies return a list of all the networkpolicies in a specific namespace
func (c *Client) NetworkPolicies(namespace string) (*networking.NetworkPolicyList, error) {
	nps, err := c.kubeClient.Networking().NetworkPolicies(namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("Couldn't get nertworkpolicies list : %s", err)
	}
	return nps, nil
}

// KubeClient returns the Kubernetes ClientSet
func (c *Client) KubeClient() kubernetes.Interface {
	return c.kubeClient
}
