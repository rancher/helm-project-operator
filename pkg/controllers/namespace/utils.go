package namespace

import (
	"fmt"

	"github.com/rancher/wrangler/pkg/apply"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func (h *handler) configureApplyForNamespace(namespace *v1.Namespace) apply.Apply {
	return h.apply.
		WithOwner(namespace).
		// Why do we need the release name?
		// To ensure that we don't override the set created by another instance of the Project Operator
		// running under a different release name operating on the same project registration namespace
		WithSetID(fmt.Sprintf("%s-%s-data", namespace.Name, h.opts.ReleaseName))
}

func (h *handler) getProjectIDFromNamespaceLabels(namespace *v1.Namespace) (string, bool) {
	labels := namespace.GetLabels()
	if labels == nil {
		return "", false
	}
	projectID, namespaceInProject := labels[h.opts.ProjectLabel]
	return projectID, namespaceInProject
}

func (h *handler) enqueueProjectHelmChartsForNamespace(namespace *v1.Namespace) error {
	projectHelmCharts, err := h.projectHelmChartCache.List(namespace.Name, labels.Everything())
	if err != nil {
		return err
	}
	for _, projectHelmChart := range projectHelmCharts {
		h.projectHelmCharts.Enqueue(projectHelmChart.Namespace, projectHelmChart.Name)
	}
	return nil
}
