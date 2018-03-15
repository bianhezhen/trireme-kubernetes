package resolver

import (
	kubecache "k8s.io/client-go/tools/cache"
)

// NamespaceWatcher implements the policy for a specific Namespace
type NamespaceWatcher struct {
	namespace            string
	podStore             kubecache.Store
	podController        kubecache.Controller
	podControllerStop    chan struct{}
	policyStore          kubecache.Store
	policyController     kubecache.Controller
	policyControllerStop chan struct{}
}

// NewNamespaceWatcher initialize a new NamespaceWatcher that watches the Pod and
// Networkpolicy events on the specific namespace passed in parameter.
func NewNamespaceWatcher(namespace string, podStore kubecache.Store, podController kubecache.Controller, podControllerStop chan struct{},
	policyStore kubecache.Store, policyController kubecache.Controller, policyControllerStop chan struct{}) *NamespaceWatcher {

	namespaceWatcher := &NamespaceWatcher{
		namespace:            namespace,
		podStore:             podStore,
		podController:        podController,
		podControllerStop:    podControllerStop,
		policyStore:          policyStore,
		policyController:     policyController,
		policyControllerStop: policyControllerStop,
	}

	return namespaceWatcher
}

func (n *NamespaceWatcher) stopWatchingNamespace() {
	n.podControllerStop <- struct{}{}
	n.policyControllerStop <- struct{}{}
}
