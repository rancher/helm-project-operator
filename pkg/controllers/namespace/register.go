package namespace

import (
	"sync"

	corev1 "k8s.io/api/core/v1"
)

// NamespaceGetter gets a namespace that has been stored in a register
type NamespaceGetter interface {
	// Has implies that the namespace has been registered
	Has(name string) bool

	// Get retrieves a registered namespace
	Get(name string) (*corev1.Namespace, bool)
}

// NamespaceRegister can store namespace references and get them
type NamespaceRegister interface {
	NamespaceGetter

	// Set registers a namespace
	Set(namespace *corev1.Namespace)

	// Delete unregisters a namespace
	Delete(namespace *corev1.Namespace)
}

// NewRegister returns a new register that can track and get namespaces
func NewRegister() NamespaceRegister {
	return &namespaceRegister{
		namespaceMap: make(map[string]*corev1.Namespace),
	}
}

type namespaceRegister struct {
	namespaceMap map[string]*corev1.Namespace
	mapLock      sync.RWMutex
}

// Has implies that the namespace has been registered
func (r *namespaceRegister) Has(name string) bool {
	r.mapLock.RLock()
	defer r.mapLock.RUnlock()
	_, exists := r.namespaceMap[name]
	return exists
}

// Get retrieves a registered namespace
func (r *namespaceRegister) Get(name string) (*corev1.Namespace, bool) {
	r.mapLock.RLock()
	defer r.mapLock.RUnlock()
	ns, exists := r.namespaceMap[name]
	if !exists {
		return nil, false
	}
	return ns, true
}

// Set registers a namespace
func (r *namespaceRegister) Set(namespace *corev1.Namespace) {
	r.mapLock.Lock()
	defer r.mapLock.Unlock()
	r.namespaceMap[namespace.Name] = namespace
}

// Delete unregisters a namespace
func (r *namespaceRegister) Delete(namespace *corev1.Namespace) {
	r.mapLock.Lock()
	defer r.mapLock.Unlock()
	delete(r.namespaceMap, namespace.Name)
}
