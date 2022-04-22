package namespace

import (
	"fmt"
	"strings"

	"github.com/aiyengar2/helm-project-operator/pkg/controllers/common"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (h *handler) getProjectRegistrationNamespace(projectID string, namespace *v1.Namespace) *v1.Namespace {
	projectIDWithClusterID := projectID
	if len(h.opts.ClusterID) > 0 {
		projectIDWithClusterID = fmt.Sprintf("%s:%s", h.opts.ClusterID, projectID)
	}

	return &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf(common.ProjectRegistrationNamespaceFmt, projectID),
			Annotations: map[string]string{
				h.opts.ProjectLabel: projectIDWithClusterID,
			},
			Labels: map[string]string{
				common.HelmProjectOperatedLabel: "true",
				h.opts.ProjectLabel:             projectID,
			},
		},
	}
}

func (h *handler) getConfigMap(namespace *v1.Namespace) *v1.ConfigMap {
	return &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      h.getConfigMapName(),
			Namespace: namespace.Name,
			Labels: map[string]string{
				common.HelmProjectOperatedLabel: "true",
			},
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
