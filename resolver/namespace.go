package resolver

import (
	kubecache "k8s.io/client-go/tools/cache"
)

// NamespaceWatcher implements the policy for a specific Namespace
type NamespaceWatcher struct {
	namespace            string
	policyStore          kubecache.Store
	policyController     kubecache.Controller
	policyControllerStop chan struct{}
}

// NewNamespaceWatcher initialize a new NamespaceWatcher that watches the Pod and
// Networkpolicy events on the specific namespace passed in parameter.
func NewNamespaceWatcher(namespace string,
	policyStore kubecache.Store, policyController kubecache.Controller, policyControllerStop chan struct{}) *NamespaceWatcher {

	namespaceWatcher := &NamespaceWatcher{
		namespace:            namespace,
		policyStore:          policyStore,
		policyController:     policyController,
		policyControllerStop: policyControllerStop,
	}

	return namespaceWatcher
}

func (n *NamespaceWatcher) stopWatchingNamespace() {
	n.policyControllerStop <- struct{}{}
}
