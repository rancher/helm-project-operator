package namespace

import (
	"fmt"
	"strings"

	"github.com/aiyengar2/helm-project-operator/pkg/controllers/common"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Note: each resource created here should have a resolver set in resolvers.go
// The only exception is namespaces since those are handled by the main controller OnChange

func (h *handler) getProjectRegistrationNamespace(projectID string, isOrphaned bool, namespace *corev1.Namespace) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:        fmt.Sprintf(common.ProjectRegistrationNamespaceFmt, projectID),
			Annotations: common.GetProjectNamespaceAnnotations(projectID, h.opts.ProjectLabel, h.opts.ClusterID),
			Labels:      common.GetProjectNamespaceLabels(projectID, h.opts.ProjectLabel, projectID, isOrphaned),
		},
	}
}

func (h *handler) getConfigMap(projectID string, namespace *corev1.Namespace) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      h.getConfigMapName(),
			Namespace: namespace.Name,
			Labels:    common.GetCommonLabels(projectID),
		},
		Data: map[string]string{
			"values.yaml":    h.valuesYaml,
			"questions.yaml": h.questionsYaml,
		},
	}
}

func (h *handler) getConfigMapName() string {
	return strings.ReplaceAll(h.opts.HelmApiVersion, "/", ".")
}
