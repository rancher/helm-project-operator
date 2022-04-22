package namespace

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

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
