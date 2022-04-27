package namespace

import (
	"sync"

	corev1 "k8s.io/api/core/v1"
)

type NamespaceGetter interface {
	Has(name string) bool
	Get(name string) (*corev1.Namespace, bool)
}

type NamespaceRegister interface {
	NamespaceGetter
	Set(namespace *corev1.Namespace)
	Delete(namespace *corev1.Namespace)
}

func NewRegister() NamespaceRegister {
	return &namespaceRegister{
		namespaceMap: make(map[string]*corev1.Namespace),
	}
}

type namespaceRegister struct {
	namespaceMap map[string]*corev1.Namespace
	mapLock      sync.RWMutex
}

func (r *namespaceRegister) Has(name string) bool {
	r.mapLock.RLock()
	defer r.mapLock.RUnlock()
	_, exists := r.namespaceMap[name]
	return exists
}

func (r *namespaceRegister) Get(name string) (*corev1.Namespace, bool) {
	r.mapLock.RLock()
	defer r.mapLock.RUnlock()
	ns, exists := r.namespaceMap[name]
	if !exists {
		return nil, false
	}
	return ns, true
}

func (r *namespaceRegister) Set(namespace *corev1.Namespace) {
	r.mapLock.Lock()
	defer r.mapLock.Unlock()
	r.namespaceMap[namespace.Name] = namespace
}

func (r *namespaceRegister) Delete(namespace *corev1.Namespace) {
	r.mapLock.Lock()
	defer r.mapLock.Unlock()
	delete(r.namespaceMap, namespace.Name)
}
