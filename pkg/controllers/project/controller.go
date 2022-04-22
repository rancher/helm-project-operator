package project

import (
	"context"
	"fmt"

	helmlocker "github.com/aiyengar2/helm-locker/pkg/generated/controllers/helm.cattle.io/v1alpha1"
	"github.com/aiyengar2/helm-project-operator/pkg/apis/helm.cattle.io/v1alpha1"
	"github.com/aiyengar2/helm-project-operator/pkg/controllers/common"
	"github.com/aiyengar2/helm-project-operator/pkg/controllers/namespace"
	helmproject "github.com/aiyengar2/helm-project-operator/pkg/generated/controllers/helm.cattle.io/v1alpha1"
	"github.com/k3s-io/helm-controller/pkg/controllers/chart"
	helm "github.com/k3s-io/helm-controller/pkg/generated/controllers/helm.cattle.io/v1"
	"github.com/rancher/wrangler/pkg/apply"
	corecontrollers "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/generic"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	DefaultJobImage = chart.DefaultJobImage
)

type handler struct {
	systemNamespace       string
	opts                  common.Options
	apply                 apply.Apply
	projectHelmCharts     helmproject.ProjectHelmChartController
	projectHelmChartCache helmproject.ProjectHelmChartCache
	helmCharts            helm.HelmChartController
	helmReleases          helmlocker.HelmReleaseController
	namespaces            corecontrollers.NamespaceController
	namespaceCache        corecontrollers.NamespaceCache
	configmaps            corecontrollers.ConfigMapController
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
	namespaces corecontrollers.NamespaceController,
	namespaceCache corecontrollers.NamespaceCache,
	configmaps corecontrollers.ConfigMapController,
	projectGetter namespace.ProjectGetter,
) {

	apply = apply.
		WithSetID("project-helm-chart-applier").
		WithCacheTypes(
			helmCharts,
			helmReleases,
			namespaces).
		WithNoDeleteGVK(namespaces.GroupVersionKind())

	h := &handler{
		systemNamespace:       systemNamespace,
		opts:                  opts,
		apply:                 apply,
		projectHelmCharts:     projectHelmCharts,
		projectHelmChartCache: projectHelmChartCache,
		helmCharts:            helmCharts,
		helmReleases:          helmReleases,
		namespaces:            namespaces,
		namespaceCache:        namespaceCache,
		configmaps:            configmaps,
		projectGetter:         projectGetter,
	}

	h.initIndexers()

	h.initResolvers(ctx)

	helmproject.RegisterProjectHelmChartGeneratingHandler(ctx,
		projectHelmCharts,
		apply,
		"",
		"project-helm-chart-registration",
		h.OnChange,
		&generic.GeneratingHandlerOptions{
			AllowClusterScoped: true,
		})

	if len(h.opts.ProjectLabel) != 0 && len(h.opts.SystemProjectLabelValue) != 0 {
		// OnRemove logic here ensures that release namespaces are marked as orphaned on removing all ProjectHelmCharts
		// However, release namespaces are only created if both --project-label and --system-project-label-value are provided,
		// so unless both are provided, we do not need to register this handler
		projectHelmCharts.OnRemove(ctx, "ensure-project-release-namespace-orphaned", h.OnRemove)
	}

	err := h.initRemoveCleanupLabels()
	if err != nil {
		logrus.Fatal(err)
	}
}

func (h *handler) shouldHandle(projectHelmChart *v1alpha1.ProjectHelmChart) (bool, error) {
	if projectHelmChart == nil {
		return false, nil
	}
	isProjectRegistrationNamespace, err := h.projectGetter.IsProjectRegistrationNamespace(projectHelmChart.Namespace)
	if err != nil {
		return false, err
	}
	if !isProjectRegistrationNamespace {
		// only watching resources in registered namespaces
		return false, nil
	}
	if projectHelmChart.Spec.HelmApiVersion != h.opts.HelmApiVersion {
		// only watch resources with the HelmAPIVersion this controller was configured with
		return false, nil
	}
	return true, nil
}

func (h *handler) OnChange(projectHelmChart *v1alpha1.ProjectHelmChart, projectHelmChartStatus v1alpha1.ProjectHelmChartStatus) ([]runtime.Object, v1alpha1.ProjectHelmChartStatus, error) {
	var objs []runtime.Object

	// initial checks to see if we should handle this
	shouldHandle, err := h.shouldHandle(projectHelmChart)
	if err != nil {
		return nil, projectHelmChartStatus, err
	}
	if !shouldHandle {
		return nil, projectHelmChartStatus, nil
	}
	if projectHelmChart.DeletionTimestamp != nil {
		return nil, projectHelmChartStatus, nil
	}

	// handle charts with cleanup label
	if h.shouldCleanup(projectHelmChart) {
		projectHelmChartStatus = h.getCleanupStatus(projectHelmChart, projectHelmChartStatus)
		logrus.Infof("Cleaning up HelmChart and HelmRelease for ProjectHelmChart %s/%s", projectHelmChart.Namespace, projectHelmChart.Name)
		return nil, projectHelmChartStatus, nil
	}

	// get information about the projectHelmChart
	projectID, err := h.getProjectID(projectHelmChart)
	if err != nil {
		return nil, projectHelmChartStatus, err
	}
	releaseNamespace, releaseName := h.getReleaseNamespaceAndName(projectHelmChart)

	// set basic statuses
	projectHelmChartStatus.SystemNamespace = h.systemNamespace
	projectHelmChartStatus.ReleaseNamespace = releaseNamespace
	projectHelmChartStatus.ReleaseName = releaseName

	// gather target project namespaces
	targetProjectNamespaces, err := h.projectGetter.GetTargetProjectNamespaces(projectHelmChart)
	if err != nil {
		projectHelmChartStatus = h.getNoTargetNamespacesStatus(projectHelmChart, projectHelmChartStatus)
		return nil, projectHelmChartStatus, fmt.Errorf("unable to get target project namespces for projectHelmChart %s/%s", projectHelmChart.Namespace, projectHelmChart.Name)
	}
	if len(targetProjectNamespaces) == 0 {
		projectReleaseNamespace := h.getProjectReleaseNamespace(projectID, projectHelmChart)
		if projectReleaseNamespace != nil {
			// always patch the projectReleaseNamespace to orphaned if it exists
			projectReleaseNamespace.Labels[common.HelmProjectOperatedOrphanedLabel] = "true"
			objs = append(objs, projectReleaseNamespace)
		}
		projectHelmChartStatus = h.getNoTargetNamespacesStatus(projectHelmChart, projectHelmChartStatus)
		return objs, projectHelmChartStatus, nil
	}

	if releaseNamespace != h.systemNamespace && releaseNamespace != projectHelmChart.Namespace {
		// need to add release namespace to list of objects to be created
		projectReleaseNamespace := h.getProjectReleaseNamespace(projectID, projectHelmChart)
		objs = append(objs, projectReleaseNamespace)
		// need to add auto-generated release namespace to target namespaces
		targetProjectNamespaces = append(targetProjectNamespaces, releaseNamespace)
	}
	projectHelmChartStatus.TargetNamespaces = targetProjectNamespaces

	// get values.yaml from ProjectHelmChart spec and default overrides
	values := h.getValues(projectHelmChart, projectID, targetProjectNamespaces)
	valuesContentBytes, err := values.ToYAML()
	if err != nil {
		projectHelmChartStatus = h.getValuesParseErrorStatus(projectHelmChart, projectHelmChartStatus, err)
		return nil, projectHelmChartStatus, fmt.Errorf("unable to marshall spec.values of %s/%s: %s", projectHelmChart.Namespace, projectHelmChart.Name, err)
	}

	// get final status from ConfigMap and deploy
	projectHelmChartStatus = h.getDeployedStatus(projectHelmChart, projectHelmChartStatus)
	objs = append(objs,
		h.getHelmChart(string(valuesContentBytes), projectHelmChart),
		h.getHelmRelease(projectHelmChart),
	)
	return objs, projectHelmChartStatus, nil
}

func (h *handler) OnRemove(key string, projectHelmChart *v1alpha1.ProjectHelmChart) (*v1alpha1.ProjectHelmChart, error) {
	// initial checks to see if we should handle this
	shouldHandle, err := h.shouldHandle(projectHelmChart)
	if err != nil {
		return projectHelmChart, err
	}
	if !shouldHandle {
		return projectHelmChart, nil
	}
	if projectHelmChart.DeletionTimestamp == nil {
		return nil, nil
	}

	// get information about the projectHelmChart
	projectID, err := h.getProjectID(projectHelmChart)
	if err != nil {
		return projectHelmChart, err
	}

	// mark as orphaned; if another ProjectHelmChart exists in this namespace, it will automatically remove the orphaned label on enqueuing
	// the namespace since that will enqueue all ProjectHelmCharts associated with it
	projectReleaseNamespace := h.getProjectReleaseNamespace(projectID, projectHelmChart)
	projectReleaseNamespace.Labels[common.HelmProjectOperatedOrphanedLabel] = "true"

	err = h.apply.ApplyObjects(projectReleaseNamespace)
	if err != nil {
		return projectHelmChart, fmt.Errorf("unable to add orphaned annotation to project release namespace %s", projectReleaseNamespace.Name)
	}
	return projectHelmChart, nil
}
