package project

import (
	"fmt"

	helmlockerapi "github.com/aiyengar2/helm-locker/pkg/apis/helm.cattle.io/v1alpha1"
	"github.com/aiyengar2/helm-project-operator/pkg/apis/helm.cattle.io/v1alpha1"
	"github.com/aiyengar2/helm-project-operator/pkg/controllers/common"
	helmapi "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (h *handler) getHelmChart(valuesContent string, projectHelmChart *v1alpha1.ProjectHelmChart) *helmapi.HelmChart {
	// must be in system namespace since helm controllers are configured to only watch one namespace
	jobImage := DefaultJobImage
	if len(h.opts.HelmJobImage) > 0 {
		jobImage = h.opts.HelmJobImage
	}
	releaseNamespace, releaseName := h.getReleaseNamespaceAndName(projectHelmChart)
	helmChart := helmapi.NewHelmChart(h.systemNamespace, releaseName, helmapi.HelmChart{
		Spec: helmapi.HelmChartSpec{
			TargetNamespace: releaseNamespace,
			Chart:           releaseName,
			JobImage:        jobImage,
			ChartContent:    h.opts.ChartContent,
			ValuesContent:   valuesContent,
		},
	})
	helmChart.SetLabels(getLabels(projectHelmChart))
	return helmChart
}

func (h *handler) getHelmRelease(projectHelmChart *v1alpha1.ProjectHelmChart) *helmlockerapi.HelmRelease {
	// must be in system namespace since helmlocker controllers are configured to only watch one namespace
	releaseNamespace, releaseName := h.getReleaseNamespaceAndName(projectHelmChart)
	helmRelease := helmlockerapi.NewHelmRelease(h.systemNamespace, releaseName, helmlockerapi.HelmRelease{
		Spec: helmlockerapi.HelmReleaseSpec{
			Release: helmlockerapi.ReleaseKey{
				Namespace: releaseNamespace,
				Name:      releaseName,
			},
		},
	})
	helmRelease.SetLabels(getLabels(projectHelmChart))
	return helmRelease
}

func (h *handler) getProjectReleaseNamespace(projectID string, projectHelmChart *v1alpha1.ProjectHelmChart) *v1.Namespace {
	releaseNamespace, _ := h.getReleaseNamespaceAndName(projectHelmChart)
	if releaseNamespace == h.systemNamespace || releaseNamespace == projectHelmChart.Namespace {
		return nil
	}
	// Project Release Namespace is only created if ProjectLabel and SystemProjectLabelValue are specified
	// It will always be created in the system project
	systemProjectIDWithClusterID := h.opts.SystemProjectLabelValue
	if len(h.opts.ClusterID) > 0 {
		systemProjectIDWithClusterID = fmt.Sprintf("%s:%s", h.opts.ClusterID, h.opts.SystemProjectLabelValue)
	}
	projectReleaseNamespace := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: releaseNamespace,
			Annotations: map[string]string{
				// auto-imports the project into the system project for RBAC stuff
				h.opts.ProjectLabel: systemProjectIDWithClusterID,
			},
			Labels: map[string]string{
				common.HelmProjectOperatedLabel: "true",
				// note: this annotation exists so that it's possible to define namespaceSelectors
				// that select both all namespaces in the project AND the namespace the release resides in
				// by selecting namespaces that have either of the following labels:
				// - h.opts.ProjectLabel: projectID
				// - helm.cattle.io/projectId: projectID
				common.HelmProjectOperatorProjectLabel: projectID,

				h.opts.ProjectLabel: h.opts.SystemProjectLabelValue,
			},
		},
	}
	return projectReleaseNamespace
}
