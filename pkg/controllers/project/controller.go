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
	corecontrollers "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/generic"
	"github.com/rancher/wrangler/pkg/relatedresource"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	namespaceCache        corecontrollers.NamespaceCache
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
		namespaceCache:        namespaceCache,
		projectGetter:         projectGetter,
	}

	helmproject.RegisterProjectHelmChartGeneratingHandler(ctx,
		projectHelmCharts,
		apply,
		"",
		"project-helm-chart-registration",
		h.OnChange,
		&generic.GeneratingHandlerOptions{
			AllowClusterScoped: true,
		})

	projectHelmCharts.OnRemove(ctx, "remove-project-helm-chart", h.OnRemove)

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

	projectID, err := h.getProjectID(projectHelmChart)
	if err != nil {
		return nil, projectHelmChartStatus, err
	}

	projectReleaseNamespace := h.getProjectReleaseNamespace(projectID, projectHelmChart)

	targetProjectNamespaces, err := h.projectGetter.GetTargetProjectNamespaces(projectHelmChart)
	if err != nil {
		return nil, projectHelmChartStatus, fmt.Errorf("unable to get target project namespces for projectHelmChart %s/%s", projectHelmChart.Namespace, projectHelmChart.Name)
	}
	if len(targetProjectNamespaces) == 0 {
		projectHelmChartStatus.ProjectHelmChartStatus = "NoTargetProjectNamespacesExist"
		projectHelmChartStatus.ProjectHelmChartStatusMessage = "There are no project namespaces to deploy a ProjectHelmChart."
		projectHelmChartStatus.ProjectNamespaces = nil
		projectHelmChartStatus.ProjectSystemNamespace = ""
		projectHelmChartStatus.ProjectReleaseNamespace = ""
		var objs []runtime.Object
		if projectReleaseNamespace != nil {
			// always patch the projectReleaseNamespace to orphaned if it exists
			projectReleaseNamespace.Labels[common.HelmProjectOperatedOrphanedLabel] = "true"
			objs = append(objs, projectReleaseNamespace)
		}
		return objs, projectHelmChartStatus, nil
	} else {
		projectHelmChartStatus.ProjectHelmChartStatus = "ValidatedNamespaces"
		projectHelmChartStatus.ProjectHelmChartStatusMessage = "ProjectHelmChart targets a valid set of namespaces."
		projectHelmChartStatus.ProjectSystemNamespace = h.systemNamespace
		projectHelmChartStatus.ProjectReleaseNamespace = h.systemNamespace
		if projectReleaseNamespace != nil {
			// ensure it is added to targets and updated on the status
			targetProjectNamespaces = append(targetProjectNamespaces, projectReleaseNamespace.Name)
			projectHelmChartStatus.ProjectReleaseNamespace = projectReleaseNamespace.Name
		}
		projectHelmChartStatus.ProjectNamespaces = targetProjectNamespaces
	}

	values := v1alpha1.GenericMap(data.MergeMaps(projectHelmChart.Spec.Values, map[string]interface{}{
		"global": map[string]interface{}{
			"cattle": map[string]interface{}{
				"projectNamespaces":        targetProjectNamespaces,
				"projectID":                projectID,
				"systemProjectID":          h.opts.SystemProjectLabelValue,
				"projectNamespaceSelector": h.getProjectNamespaceSelector(projectHelmChart, projectID),
			},
		},
	}))
	valuesContentBytes, err := values.ToYAML()
	if err != nil {
		return nil, projectHelmChartStatus, fmt.Errorf("unable to marshall spec.values of %s/%s: %s", projectHelmChart.Namespace, projectHelmChart.Name, err)
	}

	projectHelmChartStatus.ProjectHelmChartStatus = "Validated"
	projectHelmChartStatus.ProjectHelmChartStatusMessage = "ProjectHelmChart has valid values and target namespaces. HelmChart and HelmRelease should be deployed."

	var objs []runtime.Object
	if projectReleaseNamespace != nil {
		// add only if necessary
		objs = []runtime.Object{projectReleaseNamespace}
	}
	return append(objs,
		h.getHelmChart(string(valuesContentBytes), projectHelmChart),
		h.getHelmRelease(projectHelmChart),
	), projectHelmChartStatus, nil
}

func (h *handler) OnRemove(key string, projectHelmChart *v1alpha1.ProjectHelmChart) (*v1alpha1.ProjectHelmChart, error) {
	if len(h.opts.ProjectLabel) == 0 || len(h.opts.SystemProjectLabelValue) == 0 {
		// nothing to do
		return projectHelmChart, nil
	}
	// patch the project release namespace with the orphaned annotation
	projectID, err := h.getProjectID(projectHelmChart)
	if err != nil {
		return projectHelmChart, err
	}

	// get and mark as orphaned
	projectReleaseNamespace := h.getProjectReleaseNamespace(projectID, projectHelmChart)
	projectReleaseNamespace.Labels[common.HelmProjectOperatedOrphanedLabel] = "true"

	err = h.apply.ApplyObjects(projectReleaseNamespace)
	if err != nil {
		return projectHelmChart, fmt.Errorf("unable to add orphaned annotation to project release namespace %s", projectReleaseNamespace.Name)
	}
	return projectHelmChart, nil
}

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
	if releaseNamespace == h.systemNamespace {
		return nil
	}
	// Project Release Namespace is only created if ProjectLabel and SystemProjectLabelValue are specified
	// It will always be created in the system project (by annotation) in order to provide least privileges to Project Owners
	// But it will be selectable by workloads targeting the project (by label) since it still has the original project ID
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

func getLabels(projectHelmChart *v1alpha1.ProjectHelmChart) map[string]string {
	return map[string]string{
		common.HelmProjectOperatedLabel: "true",
	}
}

func (h *handler) getProjectID(projectHelmChart *v1alpha1.ProjectHelmChart) (string, error) {
	if len(h.opts.ProjectLabel) == 0 {
		return "", nil
	}
	projectRegistrationNamespace, err := h.namespaceCache.Get(projectHelmChart.Namespace)
	if err != nil {
		return "", fmt.Errorf("unable to parse projectID for projectHelmChart %s/%s: %s", projectHelmChart.Namespace, projectHelmChart.Name, err)
	}
	projectID, ok := projectRegistrationNamespace.Labels[h.opts.ProjectLabel]
	if !ok {
		return "", nil
	}
	return projectID, nil
}

func (h *handler) getReleaseNamespaceAndName(projectHelmChart *v1alpha1.ProjectHelmChart) (string, string) {
	if len(h.opts.ProjectLabel) == 0 {
		// HelmCharts, HelmReleases, and ProjectHelmCharts will be in the same namespace and project release namespaces are not created
		// Solution: use the projectHelmChart name as the differentiator per release
		return h.systemNamespace, fmt.Sprintf("%s/%s", projectHelmChart.Name, h.opts.ReleaseName)
	}
	if len(h.opts.SystemProjectLabelValue) == 0 {
		// ProjectLabel exists, which means that we are creating ProjectHelmCharts in different namespaces that HelmCharts or HelmReleases
		// However, project release namespaces are not created, so all Helm chart deployments will be in the system namespace
		// Solution: use projectHelmChart namespace as the differentiator per release
		return h.systemNamespace, fmt.Sprintf("%s/%s", projectHelmChart.Namespace, h.opts.ReleaseName)
	}
	// ProjectHelmCharts will be in dedicated project registration namespaces in the designated Project
	// HelmCharts and HelmReleases will be in the systemNamespace
	// Helm deployments will go to dedicated project release namespaces in the System project
	// Solution: use the name of the dedicateed project release namespace as the differentiator for each HelmChart and HelmRelease
	projectReleaseNamespaceName := fmt.Sprintf("%s-%s", projectHelmChart.Namespace, h.opts.ReleaseName)
	return projectReleaseNamespaceName, projectReleaseNamespaceName
}

func (h *handler) getProjectNamespaceSelector(projectHelmChart *v1alpha1.ProjectHelmChart, projectID string) map[string]interface{} {
	if len(h.opts.ProjectLabel) == 0 {
		// Use the projectHelmChart selector as the namespaceSelector
		if projectHelmChart.Spec.ProjectNamespaceSelector == nil {
			return map[string]interface{}{}
		}
		return map[string]interface{}{
			"matchLabels":      projectHelmChart.Spec.ProjectNamespaceSelector.MatchLabels,
			"matchExpressions": projectHelmChart.Spec.ProjectNamespaceSelector.MatchExpressions,
		}
	}
	if len(h.opts.SystemProjectLabelValue) == 0 {
		// Release namespace is not created, so use namespaceSelector provided tied to projectID
		return map[string]interface{}{
			"matchLabels": map[string]string{
				h.opts.ProjectLabel: projectID,
			},
		}
	}
	// use the HelmProjectOperated label
	return map[string]interface{}{
		"matchLabels": map[string]string{
			common.HelmProjectOperatorProjectLabel: projectID,
		},
	}
}
