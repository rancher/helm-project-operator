package project

import (
	"context"
	"fmt"

	"github.com/k3s-io/helm-controller/pkg/controllers/chart"
	k3shelmcontroller "github.com/k3s-io/helm-controller/pkg/generated/controllers/helm.cattle.io/v1"
	helmlockercontroller "github.com/rancher/helm-locker/pkg/generated/controllers/helm.cattle.io/v1alpha1"
	v1alpha1 "github.com/rancher/helm-project-operator/pkg/apis/helm.cattle.io/v1alpha1"
	"github.com/rancher/helm-project-operator/pkg/controllers/common"
	"github.com/rancher/helm-project-operator/pkg/controllers/namespace"
	helmprojectcontroller "github.com/rancher/helm-project-operator/pkg/generated/controllers/helm.cattle.io/v1alpha1"
	"github.com/rancher/wrangler/pkg/apply"
	corecontroller "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	rbaccontroller "github.com/rancher/wrangler/pkg/generated/controllers/rbac/v1"
	"github.com/rancher/wrangler/pkg/generic"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	DefaultJobImage = chart.DefaultJobImage
)

type handler struct {
	systemNamespace         string
	opts                    common.Options
	valuesOverride          v1alpha1.GenericMap
	apply                   apply.Apply
	projectHelmCharts       helmprojectcontroller.ProjectHelmChartController
	projectHelmChartCache   helmprojectcontroller.ProjectHelmChartCache
	configmaps              corecontroller.ConfigMapController
	configmapCache          corecontroller.ConfigMapCache
	roles                   rbaccontroller.RoleController
	roleCache               rbaccontroller.RoleCache
	clusterrolebindings     rbaccontroller.ClusterRoleBindingController
	clusterrolebindingCache rbaccontroller.ClusterRoleBindingCache
	helmCharts              k3shelmcontroller.HelmChartController
	helmReleases            helmlockercontroller.HelmReleaseController
	namespaces              corecontroller.NamespaceController
	namespaceCache          corecontroller.NamespaceCache
	rolebindings            rbaccontroller.RoleBindingController
	rolebindingCache        rbaccontroller.RoleBindingCache
	projectGetter           namespace.ProjectGetter
}

func Register(
	ctx context.Context,
	systemNamespace string,
	opts common.Options,
	valuesOverride v1alpha1.GenericMap,
	apply apply.Apply,
	projectHelmCharts helmprojectcontroller.ProjectHelmChartController,
	projectHelmChartCache helmprojectcontroller.ProjectHelmChartCache,
	configmaps corecontroller.ConfigMapController,
	configmapCache corecontroller.ConfigMapCache,
	roles rbaccontroller.RoleController,
	roleCache rbaccontroller.RoleCache,
	clusterrolebindings rbaccontroller.ClusterRoleBindingController,
	clusterrolebindingCache rbaccontroller.ClusterRoleBindingCache,
	helmCharts k3shelmcontroller.HelmChartController,
	helmReleases helmlockercontroller.HelmReleaseController,
	namespaces corecontroller.NamespaceController,
	namespaceCache corecontroller.NamespaceCache,
	rolebindings rbaccontroller.RoleBindingController,
	rolebindingCache rbaccontroller.RoleBindingCache,
	projectGetter namespace.ProjectGetter,
) {

	apply = apply.
		WithSetID("project-helm-chart-applier").
		WithCacheTypes(
			helmCharts,
			helmReleases,
			namespaces,
			rolebindings).
		WithNoDeleteGVK(namespaces.GroupVersionKind())

	h := &handler{
		systemNamespace:         systemNamespace,
		opts:                    opts,
		valuesOverride:          valuesOverride,
		apply:                   apply,
		projectHelmCharts:       projectHelmCharts,
		projectHelmChartCache:   projectHelmChartCache,
		configmaps:              configmaps,
		configmapCache:          configmapCache,
		roles:                   roles,
		clusterrolebindings:     clusterrolebindings,
		clusterrolebindingCache: clusterrolebindingCache,
		roleCache:               roleCache,
		helmCharts:              helmCharts,
		helmReleases:            helmReleases,
		namespaces:              namespaces,
		namespaceCache:          namespaceCache,
		rolebindings:            rolebindings,
		rolebindingCache:        rolebindingCache,
		projectGetter:           projectGetter,
	}

	h.initIndexers()

	h.initResolvers(ctx)

	helmprojectcontroller.RegisterProjectHelmChartGeneratingHandler(ctx,
		projectHelmCharts,
		apply,
		"",
		"project-helm-chart-registration",
		h.OnChange,
		&generic.GeneratingHandlerOptions{
			AllowClusterScoped: true,
		})

	// Note: why do we create an OnChange handler for an OnRemove function?
	//
	// The OnRemove handler creates an objectLifecycleAdapter that adds a Kubernetes
	// finalizer to the object in question to ensure that the handler that we run
	// gets applied **before** the object gets deleted. However, using finalizers
	// ends up adding the finalizers to all objects and, in this particular case,
	// we don't care whether the action happens **pre**-delete or **post**-delete.
	//
	// Therefore, to avoid creating any finalizers, we simply register this OnRemove
	// handler as an OnChange handler.
	//
	// Note: why can't this be handled by the GeneratingHandler?
	// The GeneratingHandler is built to not run the handler provided if the object's
	// deletion timestamp is zero (since the design of the GeneratingHandler assumes)
	// that all objects are cleaned up on deleting the parent object; the concept of
	// orphaned child objects is not part of its pattern). Therefore, we need to run
	// our OnRemove logic as a separate handler.
	projectHelmCharts.OnChange(ctx, "project-helm-chart-removal", h.OnRemove)

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
	if projectHelmChart.Spec.HelmAPIVersion != h.opts.HelmAPIVersion {
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
	if common.HasCleanupLabel(projectHelmChart) {
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

	// check if the releaseName is already tracked by another ProjectHelmChart
	projectHelmCharts, err := h.projectHelmChartCache.GetByIndex(ProjectHelmChartByReleaseName, releaseName)
	if err != nil {
		return nil, projectHelmChartStatus, fmt.Errorf("unable to get ProjectHelmCharts to verify if release is already tracked: %s", err)
	}
	for _, conflictingProjectHelmChart := range projectHelmCharts {
		if conflictingProjectHelmChart == nil {
			continue
		}
		if projectHelmChart.Name == conflictingProjectHelmChart.Name && projectHelmChart.Namespace == conflictingProjectHelmChart.Namespace {
			// looking at the same projectHelmChart that we have at hand
			continue
		}
		if len(conflictingProjectHelmChart.Status.Status) == 0 {
			// the other ProjectHelmChart hasn't been processed yet, so let it fail out whenever it is processed
			continue
		}
		if conflictingProjectHelmChart.Status.Status == "UnableToCreateHelmRelease" {
			// the other ProjectHelmChart is the one that will not be able to progress, so we can continue to update this one
			continue
		}
		// we have found another ProjectHelmChart that already exists and is tracking this release with some non-conflicting status
		err = fmt.Errorf(
			"ProjectHelmChart %s/%s already tracks release %s/%s",
			conflictingProjectHelmChart.Namespace, conflictingProjectHelmChart.Name,
			releaseName, releaseNamespace,
		)
		projectHelmChartStatus = h.getUnableToCreateHelmReleaseStatus(projectHelmChart, projectHelmChartStatus, err)
		return nil, projectHelmChartStatus, nil
	}

	// set basic statuses
	projectHelmChartStatus.SystemNamespace = h.systemNamespace
	projectHelmChartStatus.ReleaseNamespace = releaseNamespace
	projectHelmChartStatus.ReleaseName = releaseName

	// gather target project namespaces
	targetProjectNamespaces, err := h.projectGetter.GetTargetProjectNamespaces(projectHelmChart)
	if err != nil {
		return nil, projectHelmChartStatus, fmt.Errorf("unable to find project namespaces to deploy ProjectHelmChart: %s", err)
	}
	if len(targetProjectNamespaces) == 0 {
		projectReleaseNamespace := h.getProjectReleaseNamespace(projectID, true, projectHelmChart)
		if projectReleaseNamespace != nil {
			objs = append(objs, projectReleaseNamespace)
		}
		projectHelmChartStatus = h.getNoTargetNamespacesStatus(projectHelmChart, projectHelmChartStatus)
		return objs, projectHelmChartStatus, nil
	}

	if releaseNamespace != h.systemNamespace && releaseNamespace != projectHelmChart.Namespace {
		// need to add release namespace to list of objects to be created
		projectReleaseNamespace := h.getProjectReleaseNamespace(projectID, false, projectHelmChart)
		objs = append(objs, projectReleaseNamespace)
		// need to add auto-generated release namespace to target namespaces
		targetProjectNamespaces = append(targetProjectNamespaces, releaseNamespace)
	}
	projectHelmChartStatus.TargetNamespaces = targetProjectNamespaces

	// get values.yaml from ProjectHelmChart spec and default overrides
	values := h.getValues(projectHelmChart, projectID, targetProjectNamespaces)
	valuesContentBytes, err := values.ToYAML()
	if err != nil {
		err = fmt.Errorf("unable to marshall spec.values: %s", err)
		projectHelmChartStatus = h.getValuesParseErrorStatus(projectHelmChart, projectHelmChartStatus, err)
		return nil, projectHelmChartStatus, nil
	}

	// get rolebindings that need to be created in release namespace
	k8sRolesToRoleRefs, err := h.getSubjectRoleToRoleRefsFromRoles(projectHelmChart)
	if err != nil {
		return nil, projectHelmChartStatus, fmt.Errorf("unable to get release roles from project release namespace %s for %s/%s: %s", releaseNamespace, projectHelmChart.Namespace, projectHelmChart.Name, err)
	}
	k8sRolesToSubjects, err := h.getSubjectRoleToSubjectsFromBindings(projectHelmChart)
	if err != nil {
		return nil, projectHelmChartStatus, fmt.Errorf("unable to get rolebindings to default project operator roles from project registration namespace %s for %s/%s: %s", projectHelmChart.Namespace, projectHelmChart.Namespace, projectHelmChart.Name, err)
	}
	objs = append(objs,
		h.getRoleBindings(projectID, k8sRolesToRoleRefs, k8sRolesToSubjects, projectHelmChart)...,
	)

	// append the helm chart and helm release
	objs = append(objs,
		h.getHelmChart(projectID, string(valuesContentBytes), projectHelmChart),
		h.getHelmRelease(projectID, projectHelmChart),
	)

	// get dashboard values if available
	dashboardValues, err := h.getDashboardValuesFromConfigmaps(projectHelmChart)
	if err != nil {
		return nil, projectHelmChartStatus, fmt.Errorf("unable to get dashboard values from status ConfigMaps: %s", err)
	}
	if len(dashboardValues) == 0 {
		projectHelmChartStatus = h.getWaitingForDashboardValuesStatus(projectHelmChart, projectHelmChartStatus)
	} else {
		projectHelmChartStatus.DashboardValues = dashboardValues
		projectHelmChartStatus = h.getDeployedStatus(projectHelmChart, projectHelmChartStatus)
	}
	return objs, projectHelmChartStatus, nil
}

func (h *handler) OnRemove(key string, projectHelmChart *v1alpha1.ProjectHelmChart) (*v1alpha1.ProjectHelmChart, error) {
	if projectHelmChart == nil {
		return projectHelmChart, nil
	}
	if projectHelmChart.DeletionTimestamp == nil {
		// only apply on deleted objects
		return projectHelmChart, nil
	}
	// get information about the projectHelmChart
	projectID, err := h.getProjectID(projectHelmChart)
	if err != nil {
		return projectHelmChart, err
	}

	// Get orphaned release namsepace and apply it; if another ProjectHelmChart exists in this namespace, it will automatically remove
	// the orphaned label on enqueuing the namespace since that will enqueue all ProjectHelmCharts associated with it
	projectReleaseNamespace := h.getProjectReleaseNamespace(projectID, true, projectHelmChart)
	if projectReleaseNamespace == nil {
		// nothing to be done since this operator does not create project release namespaces
		return projectHelmChart, nil
	}

	// Why aren't we modifying the set ID or owner here?
	// Since this applier runs without deleting objects whose GVKs indicate that they are namespaces,
	// we don't have to worry about another controller using this same set ID (e.g. another Project Operator)
	// that will delete this projectReleaseNamespace on seeing it
	err = h.apply.ApplyObjects(projectReleaseNamespace)
	if err != nil {
		return projectHelmChart, fmt.Errorf("unable to add orphaned annotation to project release namespace %s", projectReleaseNamespace.Name)
	}
	return projectHelmChart, nil
}
