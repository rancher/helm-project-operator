package namespace

import (
	"github.com/aiyengar2/helm-project-operator/pkg/controllers/common"
	corev1 "k8s.io/api/core/v1"
)

const (
	NamespacesByProjectExcludingRegistrationID = "helm.cattle.io/namespaces-by-project-id-excluding-registration"
)

// initIndexers initializes indexers that allow for more efficient computations on related resources without relying on additional
// calls to be made to the Kubernetes API by referencing the cache instead
func (h *handler) initIndexers() {
	h.namespaceCache.AddIndexer(NamespacesByProjectExcludingRegistrationID, h.namespaceToProjectIDExcludingRegistration)
}

func (h *handler) namespaceToProjectIDExcludingRegistration(namespace *corev1.Namespace) ([]string, error) {
	if h.isSystemNamespace(namespace) {
		return nil, nil
	}
	if h.isProjectRegistrationNamespace(namespace) {
		return nil, nil
	}
	if namespace.Labels[common.HelmProjectOperatedLabel] == "true" {
		// always ignore Helm Project Operated namespaces since those are only
		// to be scoped to namespaces that are project registration namespaces
		return nil, nil
	}
	projectID, inProject := h.getProjectIDFromNamespaceLabels(namespace)
	if !inProject {
		// nothing to do
		return nil, nil
	}
	return []string{projectID}, nil
}
