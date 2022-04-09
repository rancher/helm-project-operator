package project

import (
	"context"
	"fmt"

	helmlockerapi "github.com/aiyengar2/helm-locker/pkg/apis/helm.cattle.io/v1alpha1"
	helmlocker "github.com/aiyengar2/helm-locker/pkg/generated/controllers/helm.cattle.io/v1alpha1"
	"github.com/aiyengar2/helm-project-operator/pkg/apis/helm.cattle.io/v1alpha1"
	"github.com/aiyengar2/helm-project-operator/pkg/controllers/common"
	"github.com/aiyengar2/helm-project-operator/pkg/controllers/namespace"
	helmproject "github.com/aiyengar2/helm-project-operator/pkg/generated/controllers/helm.cattle.io/v1alpha1"
	helmapi "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	"github.com/k3s-io/helm-controller/pkg/controllers/chart"
	helm "github.com/k3s-io/helm-controller/pkg/generated/controllers/helm.cattle.io/v1"
	"github.com/rancher/wrangler/pkg/apply"
	"github.com/rancher/wrangler/pkg/data"
	"github.com/rancher/wrangler/pkg/relatedresource"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	// 63 is the maximum number of characters for any resource created in the Kubernetes API
	// since we create child resources, which seem to take at least 14 characters for something like
	// "chart-values-%s", we need to ensure that the sum total of {namespace}/{name} does not exceed this value
	MaxNumberOfCharacters = 63 - 14
)

var (
	DefaultJobImage = chart.DefaultJobImage
)

type handler struct {
	systemNamespace       string
	opts                  common.Options
	projectHelmCharts     helmproject.ProjectHelmChartController
	projectHelmChartCache helmproject.ProjectHelmChartCache
	helmCharts            helm.HelmChartController
	helmReleases          helmlocker.HelmReleaseController
	projectGetter         namespace.ProjectGetter
}

func Register(
	ctx context.Context,
	systemNamespace string,
	opts common.Options,
	apply apply.Apply,
	projectHelmCharts helmproject.ProjectHelmChartController,
	projectHelmChartCache helmproject.ProjectHelmChartCache,
	helmCharts helm.HelmChartController,
	helmReleases helmlocker.HelmReleaseController,
	projectGetter namespace.ProjectGetter,
) {

	h := &handler{
		systemNamespace:       systemNamespace,
		opts:                  opts,
		projectHelmCharts:     projectHelmCharts,
		projectHelmChartCache: projectHelmChartCache,
		helmCharts:            helmCharts,
		helmReleases:          helmReleases,
		projectGetter:         projectGetter,
	}

	helmproject.RegisterProjectHelmChartGeneratingHandler(ctx,
		projectHelmCharts,
		apply.WithCacheTypes(
			helmCharts,
			helmReleases,
		),
		"",
		"project-helm-chart-registration",
		h.OnChange,
		nil)

	relatedresource.Watch(ctx, "sync-helm-resources", h.resolveProjectHelmChartOwned, projectHelmCharts, helmCharts, helmReleases)
}

func (h *handler) resolveProjectHelmChartOwned(namespace, name string, obj runtime.Object) ([]relatedresource.Key, error) {
	if namespace != h.systemNamespace {
		// only watching HelmCharts and HelmReleases in the system namespace
		return nil, nil
	}
	if obj == nil {
		return nil, nil
	}
	// Q: Why aren't we using relatedresource.OwnerResolver?
	// A: in k8s, you can't set an owner reference across namespaces, which means that when --project-label is provided
	// (where the ProjectHelmChart will be outside the systemNamespace where the HelmCharts and HelmReleases are created),
	// ownerReferences will not be set on the object. However, wrangler annotations will be set since those objects are
	// created via a wrangler apply. Therefore, we leverage those annotations to figure out which ProjectHelmChart to enqueue
	meta, err := meta.Accessor(obj)
	if err != nil {
		return nil, err
	}
	ownerNamespace, ok := meta.GetAnnotations()[apply.LabelNamespace]
	if !ok {
		return nil, nil
	}
	ownerName, ok := meta.GetAnnotations()[apply.LabelName]
	if !ok {
		return nil, nil
	}
	return []relatedresource.Key{{
		Namespace: ownerNamespace,
		Name:      ownerName,
	}}, nil
}

func (h *handler) OnChange(projectHelmChart *v1alpha1.ProjectHelmChart, projectHelmChartStatus v1alpha1.ProjectHelmChartStatus) ([]runtime.Object, v1alpha1.ProjectHelmChartStatus, error) {
	if projectHelmChart == nil {
		return nil, projectHelmChartStatus, nil
	}
	if projectHelmChart.DeletionTimestamp != nil {
		return nil, projectHelmChartStatus, nil
	}
	isProjectRegistrationNamespace, err := h.projectGetter.IsProjectRegistrationNamespace(projectHelmChart.Namespace)
	if err != nil {
		return nil, projectHelmChartStatus, err
	}
	if !isProjectRegistrationNamespace {
		// only watching resources in registered namespaces
		return nil, projectHelmChartStatus, nil
	}
	if projectHelmChart.Spec.HelmApiVersion != h.opts.HelmApiVersion {
		// only watch resources with the HelmAPIVersion this controller was configured with
		return nil, projectHelmChartStatus, nil
	}

	chartName, err := getChartName(projectHelmChart)
	if err != nil {
		return nil, projectHelmChartStatus, err
	}

	targetProjectNamespaces, err := h.projectGetter.GetTargetProjectNamespaces(projectHelmChart)
	if err != nil {
		return nil, projectHelmChartStatus, fmt.Errorf("unable to get target project namespces for projectHelmChart %s/%s", projectHelmChart.Namespace, projectHelmChart.Name)
	}
	if len(targetProjectNamespaces) == 0 {
		projectHelmChartStatus.ProjectHelmChartStatus = "NoTargetProjectNamespacesExist"
		projectHelmChartStatus.ProjectHelmChartStatusMessage = "There are no project namespaces to deploy a ProjectHelmChart."
		return nil, projectHelmChartStatus, nil
	} else {
		projectHelmChartStatus.ProjectHelmChartStatus = "ValidatedNamespaces"
		projectHelmChartStatus.ProjectNamespaces = targetProjectNamespaces
		projectHelmChartStatus.ProjectSystemNamespace = h.systemNamespace
		projectHelmChartStatus.ProjectHelmChartStatusMessage = "ProjectHelmChart targets a valid set of namespaces."
	}

	values := v1alpha1.GenericMap(data.MergeMaps(projectHelmChart.Spec.Values, map[string]interface{}{
		"global": map[string]interface{}{
			"cattle": map[string]interface{}{
				"projectNamespaces": targetProjectNamespaces,
			},
		},
	}))
	valuesContentBytes, err := values.ToYAML()
	if err != nil {
		return nil, projectHelmChartStatus, fmt.Errorf("unable to marshall spec.values of %s/%s: %s", projectHelmChart.Namespace, projectHelmChart.Name, err)
	}

	projectHelmChartStatus.ProjectHelmChartStatus = "Validated"
	projectHelmChartStatus.ProjectHelmChartStatusMessage = "ProjectHelmChart has valid values and target namespaces. HelmChart and HelmRelease should be deployed."

	return []runtime.Object{
		h.getHelmChart(chartName, string(valuesContentBytes), projectHelmChart),
		h.getHelmRelease(chartName, projectHelmChart),
	}, projectHelmChartStatus, nil
}

func getChartName(projectHelmChart *v1alpha1.ProjectHelmChart) (string, error) {
	chartName := fmt.Sprintf("%s-%s", projectHelmChart.Namespace, projectHelmChart.Name)
	if len(chartName) > MaxNumberOfCharacters {
		return "", fmt.Errorf("projectHelmChart %s/%s will create child resources that exceed the max length of characters for Kubernetes objects: chart name %s must be at most %d characters log",
			projectHelmChart.Namespace, projectHelmChart.Name, chartName, MaxNumberOfCharacters)
	}
	return chartName, nil
}

func (h *handler) getHelmChart(chartName, valuesContent string, projectHelmChart *v1alpha1.ProjectHelmChart) *helmapi.HelmChart {
	// must be in system namespace since helm controllers are configured to only watch one namespace
	jobImage := DefaultJobImage
	if len(h.opts.HelmJobImage) > 0 {
		jobImage = h.opts.HelmJobImage
	}
	helmChart := helmapi.NewHelmChart(h.systemNamespace, chartName, helmapi.HelmChart{
		Spec: helmapi.HelmChartSpec{
			Chart:           projectHelmChart.Name,
			TargetNamespace: projectHelmChart.Namespace,
			JobImage:        jobImage,
			ChartContent:    h.opts.ChartContent,
			ValuesContent:   valuesContent,
		},
	})
	helmChart.SetLabels(getLabels(projectHelmChart))
	return helmChart
}

func (h *handler) getHelmRelease(chartName string, projectHelmChart *v1alpha1.ProjectHelmChart) *helmlockerapi.HelmRelease {
	// must be in system namespace since helmlocker controllers are configured to only watch one namespace
	helmRelease := helmlockerapi.NewHelmRelease(h.systemNamespace, chartName, helmlockerapi.HelmRelease{
		Spec: helmlockerapi.HelmReleaseSpec{
			Release: helmlockerapi.ReleaseKey{
				Name:      chartName,
				Namespace: projectHelmChart.Namespace,
			},
		},
	})
	helmRelease.SetLabels(getLabels(projectHelmChart))
	return helmRelease
}

func getLabels(projectHelmChart *v1alpha1.ProjectHelmChart) map[string]string {
	return map[string]string{
		common.HelmProjectOperatedLabel: "true",
	}
}
