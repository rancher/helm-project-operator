package project

import "github.com/aiyengar2/helm-project-operator/pkg/apis/helm.cattle.io/v1alpha1"

const (
	ProjectHelmChartByReleaseName = "helm.cattle.io/project-helm-chart-by-release-name"
)

func (h *handler) initIndexers() {
	h.projectHelmChartCache.AddIndexer(ProjectHelmChartByReleaseName, h.projectHelmChartToReleaseName)
}

func (h *handler) projectHelmChartToReleaseName(projectHelmChart *v1alpha1.ProjectHelmChart) ([]string, error) {
	_, releaseName := h.getReleaseNamespaceAndName(projectHelmChart)
	return []string{releaseName}, nil
}
