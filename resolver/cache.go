package resolver

import (
	"fmt"
	"sync"

	"github.com/aporeto-inc/trireme-lib/policy"
)

type podCacheEntry struct {
	contextID string
	runtime   policy.RuntimeReader
}

// Cache keeps all the state needed for the integration.
type cache struct {
	// namespaceActivation is a map between the namespaceName and the corresponding Watcher struct.
	namespaceActivation map[string]*NamespaceWatcher
	// contextIDCache keeps a mapping between a POD/Namespace name and the corresponding contextID from Trireme.
	podCache map[string]podCacheEntry
	sync.RWMutex
}

func newCache() *cache {
	return &cache{
		namespaceActivation: map[string]*NamespaceWatcher{},
		podCache:            map[string]podCacheEntry{},
	}
}

func kubePodIdentifier(podName string, podNamespace string) string {
	return podNamespace + "/" + podName
}

func (c *cache) addPodToCache(contextID string, runtime policy.RuntimeReader, podName string, podNamespace string) {
	c.Lock()
	defer c.Unlock()
	kubeIdentifier := kubePodIdentifier(podName, podNamespace)
	c.podCache[kubeIdentifier] = podCacheEntry{
		contextID: contextID,
		runtime:   runtime,
	}
}

func (c *cache) contextIDByPodName(podName string, podNamespace string) (string, error) {
	c.Lock()
	defer c.Unlock()
	kubeIdentifier := kubePodIdentifier(podName, podNamespace)
	cacheEntry, ok := c.podCache[kubeIdentifier]
	if !ok {
		return "", fmt.Errorf("Pod %v not found in Cache", kubeIdentifier)
	}
	return cacheEntry.contextID, nil
}

func (c *cache) runtimeByPodName(podName string, podNamespace string) (policy.RuntimeReader, error) {
	c.Lock()
	defer c.Unlock()
	kubeIdentifier := kubePodIdentifier(podName, podNamespace)
	cacheEntry, ok := c.podCache[kubeIdentifier]
	if !ok {
		return nil, fmt.Errorf("Pod %v not found in Cache", kubeIdentifier)
	}
	return cacheEntry.runtime, nil
}

func (c *cache) deleteFromCacheByPodName(podName string, podNamespace string) error {
	c.Lock()
	defer c.Unlock()
	kubeIdentifier := kubePodIdentifier(podName, podNamespace)
	_, ok := c.podCache[kubeIdentifier]
	if !ok {
		return fmt.Errorf("Pod %v not found in Cache", kubeIdentifier)
	}
	delete(c.podCache, kubeIdentifier)
	return nil
}

func (c *cache) getNamespaceWatcher(namespace string) (*NamespaceWatcher, bool) {
	c.Lock()
	defer c.Unlock()
	namespaceWatcher, ok := c.namespaceActivation[namespace]
	return namespaceWatcher, ok
}

func (c *cache) activateNamespaceWatcher(namespace string, namespaceWatcher *NamespaceWatcher) {
	c.Lock()
	defer c.Unlock()
	c.namespaceActivation[namespace] = namespaceWatcher
}

func (c *cache) deactivateNamespaceWatcher(namespace string) {
	c.Lock()
	defer c.Unlock()
	namespaceWatcher, ok := c.namespaceActivation[namespace]
	if !ok {
		return
	}
	namespaceWatcher.stopWatchingNamespace()
	delete(c.namespaceActivation, namespace)
}

func (c *cache) isNamespaceActive(namespace string) bool {
	c.Lock()
	defer c.Unlock()
	_, ok := c.namespaceActivation[namespace]
	return ok
}
