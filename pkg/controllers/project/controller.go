package project

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	helmlockerapi "github.com/aiyengar2/helm-locker/pkg/apis/helm.cattle.io/v1alpha1"
	helmlocker "github.com/aiyengar2/helm-locker/pkg/generated/controllers/helm.cattle.io/v1alpha1"
	"github.com/aiyengar2/helm-project-operator/pkg/apis/helm.cattle.io/v1alpha1"
	"github.com/aiyengar2/helm-project-operator/pkg/controllers/common"
	"github.com/aiyengar2/helm-project-operator/pkg/controllers/namespace"
	"github.com/aiyengar2/helm-project-operator/pkg/controllers/rolebinding"
	helmproject "github.com/aiyengar2/helm-project-operator/pkg/generated/controllers/helm.cattle.io/v1alpha1"
	helmapi "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	"github.com/k3s-io/helm-controller/pkg/controllers/chart"
	helm "github.com/k3s-io/helm-controller/pkg/generated/controllers/helm.cattle.io/v1"
	"github.com/rancher/wrangler/pkg/apply"
	"github.com/rancher/wrangler/pkg/data"
	corecontrollers "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	rbacv1 "github.com/rancher/wrangler/pkg/generated/controllers/rbac/v1"
	"github.com/rancher/wrangler/pkg/generic"
	"github.com/rancher/wrangler/pkg/relatedresource"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	ProjectHelmChartByReleaseName = "helm.cattle.io/project-helm-chart-by-release-name"
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
	rolebindings          rbacv1.RoleBindingController
	roles                 rbacv1.RoleController
	projectGetter         namespace.ProjectGetter
	subjectRoleGetter     rolebinding.SubjectRoleGetter
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
	rolebindings rbacv1.RoleBindingController,
	roles rbacv1.RoleController,
	projectGetter namespace.ProjectGetter,
	subjectRoleGetter rolebinding.SubjectRoleGetter,
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
		rolebindings:          rolebindings,
		roles:                 roles,
		projectGetter:         projectGetter,
		subjectRoleGetter:     subjectRoleGetter,
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

	projectHelmChartCache.AddIndexer(ProjectHelmChartByReleaseName, h.projectHelmChartToReleaseName)

	relatedresource.Watch(ctx, "sync-helm-resources", h.resolveProjectHelmChartOwned, projectHelmCharts, helmCharts, helmReleases)

	relatedresource.Watch(ctx, "watch-status-configmaps", h.resolveProjectHelmChartStatusChange, projectHelmCharts, configmaps)

	relatedresource.Watch(ctx, "watch-project-helm-chart-rolebindings", h.resolveProjectHelmChartRoleBindingChange, projectHelmCharts, rolebindings)

	relatedresource.Watch(ctx, "watch-project-helm-chart-roles", h.resolveProjectHelmChartRoleChange, projectHelmCharts, roles)

	err := h.removeCleanupLabelsFromProjectHelmCharts()
	if err != nil {
		logrus.Fatal(err)
	}
}

func (h *handler) removeCleanupLabelsFromProjectHelmCharts() error {
	namespaceList, err := h.namespaces.List(metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("unable to list namespaces to remove cleanup label from all ProjectHelmCharts")
	}
	if namespaceList == nil {
		return nil
	}
	logrus.Infof("Removing cleanup label from all registered ProjectHelmCharts...")
	// ensure all ProjectHelmCharts in every namespace no longer have the cleanup label on them
	for _, ns := range namespaceList.Items {
		projectHelmChartList, err := h.projectHelmCharts.List(ns.Name, metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("unable to list ProjectHelmCharts in namespace %s to remove cleanup label", ns.Name)
		}
		if projectHelmChartList == nil {
			continue
		}
		for _, projectHelmChart := range projectHelmChartList.Items {
			if projectHelmChart.Labels == nil {
				continue
			}
			_, ok := projectHelmChart.Labels[common.HelmProjectOperatedCleanupLabel]
			if !ok {
				continue
			}
			projectHelmChartCopy := projectHelmChart.DeepCopy()
			delete(projectHelmChartCopy.Labels, common.HelmProjectOperatedCleanupLabel)
			_, err := h.projectHelmCharts.Update(projectHelmChartCopy)
			if err != nil {
				return fmt.Errorf("unable to remove cleanup label from ProjectHelmCharts %s/%s", projectHelmChart.Namespace, projectHelmChart.Name)
			}
		}
	}
	return nil
}

func (h *handler) projectHelmChartToReleaseName(projectHelmChart *v1alpha1.ProjectHelmChart) ([]string, error) {
	_, releaseName := h.getReleaseNamespaceAndName(projectHelmChart)
	return []string{releaseName}, nil
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

func (h *handler) resolveProjectHelmChartRoleBindingChange(_, name string, obj runtime.Object) ([]relatedresource.Key, error) {
	if obj == nil {
		return nil, nil
	}
	meta, err := meta.Accessor(obj)
	if err != nil {
		return nil, err
	}
	releaseName, ok := meta.GetLabels()[common.HelmProjectOperatorProjectHelmChartRoleBindingLabel]
	if !ok {
		return nil, nil
	}
	if meta.GetNamespace() != releaseName && meta.GetNamespace() != h.systemNamespace {
		// only care about rolebindings in a release namespace or the system namespace
		return nil, nil
	}
	projectHelmCharts, err := h.projectHelmChartCache.GetByIndex(ProjectHelmChartByReleaseName, releaseName)
	if err != nil {
		return nil, err
	}
	var keys []relatedresource.Key
	for _, projectHelmChart := range projectHelmCharts {
		if projectHelmChart == nil {
			continue
		}
		keys = append(keys, relatedresource.Key{
			Namespace: projectHelmChart.Namespace,
			Name:      projectHelmChart.Name,
		})
	}
	return keys, nil
}

func (h *handler) resolveProjectHelmChartRoleChange(_, name string, obj runtime.Object) ([]relatedresource.Key, error) {
	if obj == nil {
		return nil, nil
	}
	meta, err := meta.Accessor(obj)
	if err != nil {
		return nil, err
	}
	releaseName, ok := meta.GetLabels()[common.HelmProjectOperatorProjectHelmChartRoleLabel]
	if !ok {
		return nil, nil
	}
	if meta.GetNamespace() != releaseName && meta.GetNamespace() != h.systemNamespace {
		// only care about rolebindings in a release namespace or the system namespace
		return nil, nil
	}
	projectHelmCharts, err := h.projectHelmChartCache.GetByIndex(ProjectHelmChartByReleaseName, releaseName)
	if err != nil {
		return nil, err
	}
	var keys []relatedresource.Key
	for _, projectHelmChart := range projectHelmCharts {
		if projectHelmChart == nil {
			continue
		}
		keys = append(keys, relatedresource.Key{
			Namespace: projectHelmChart.Namespace,
			Name:      projectHelmChart.Name,
		})
	}
	return keys, nil
}

func (h *handler) resolveProjectHelmChartStatusChange(_, name string, obj runtime.Object) ([]relatedresource.Key, error) {
	if obj == nil {
		return nil, nil
	}
	meta, err := meta.Accessor(obj)
	if err != nil {
		return nil, err
	}
	releaseName, ok := meta.GetLabels()[common.HelmProjectOperatorDashboardValuesConfigMapLabel]
	if !ok {
		return nil, nil
	}
	if meta.GetNamespace() != releaseName && meta.GetNamespace() != h.systemNamespace {
		// only care about status configmaps in the release namespace or the system namespace
		return nil, nil
	}
	projectHelmCharts, err := h.projectHelmChartCache.GetByIndex(ProjectHelmChartByReleaseName, releaseName)
	if err != nil {
		return nil, err
	}
	var keys []relatedresource.Key
	for _, projectHelmChart := range projectHelmCharts {
		if projectHelmChart == nil {
			continue
		}
		keys = append(keys, relatedresource.Key{
			Namespace: projectHelmChart.Namespace,
			Name:      projectHelmChart.Name,
		})
	}
	return keys, nil
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
	if projectHelmChart.Labels != nil {
		_, ok := projectHelmChart.Labels[common.HelmProjectOperatedCleanupLabel]
		if ok {
			// allow cleanup to happen
			logrus.Infof("Cleaning up HelmChart and HelmRelease for ProjectHelmChart %s/%s", projectHelmChart.Namespace, projectHelmChart.Name)
			projectHelmChartStatus.ProjectHelmChartStatus = "AwaitingOperatorRedeployment"
			projectHelmChartStatus.ProjectHelmChartStatusMessage = fmt.Sprintf(
				"ProjectHelmChart was marked with label %s=true, which indicates that the resource should be cleaned up "+
					"until the Project Operator that responds to ProjectHelmCharts in %s with spec.helmApiVersion=%s "+
					"is redeployed onto the cluster. On redeployment, this label will automatically be removed by the operator.",
				common.HelmProjectOperatedCleanupLabel, projectHelmChart.Namespace, projectHelmChart.Spec.HelmApiVersion)
			projectHelmChartStatus.ProjectNamespaces = nil
			projectHelmChartStatus.ProjectSystemNamespace = ""
			projectHelmChartStatus.ProjectReleaseNamespace = ""
			projectHelmChartStatus.DashboardValues = nil
			return nil, projectHelmChartStatus, nil
		}
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
	targetProjectNamespacesWithReleaseNamespace := append(targetProjectNamespaces, projectReleaseNamespace.Name)
	if len(targetProjectNamespaces) == 0 {
		projectHelmChartStatus.ProjectHelmChartStatus = "NoTargetProjectNamespacesExist"
		projectHelmChartStatus.ProjectHelmChartStatusMessage = "There are no project namespaces to deploy a ProjectHelmChart."
		projectHelmChartStatus.ProjectNamespaces = nil
		projectHelmChartStatus.ProjectSystemNamespace = ""
		projectHelmChartStatus.ProjectReleaseNamespace = ""
		projectHelmChartStatus.DashboardValues = nil
		var objs []runtime.Object
		if projectReleaseNamespace != nil {
			// always patch the projectReleaseNamespace to orphaned if it exists
			projectReleaseNamespace.Labels[common.HelmProjectOperatedOrphanedLabel] = "true"
			objs = append(objs, projectReleaseNamespace)
		}
		return objs, projectHelmChartStatus, nil
	}
	projectHelmChartStatus.ProjectSystemNamespace = h.systemNamespace
	if projectReleaseNamespace != nil {
		// ensure it is added to targets and updated on the status
		projectHelmChartStatus.ProjectReleaseNamespace = projectReleaseNamespace.Name
		projectHelmChartStatus.ProjectNamespaces = targetProjectNamespacesWithReleaseNamespace
	} else {
		projectHelmChartStatus.ProjectReleaseNamespace = h.systemNamespace
		projectHelmChartStatus.ProjectNamespaces = targetProjectNamespaces
	}
	projectHelmChartStatus.DashboardValues = nil

	values := h.getValues(projectHelmChart, projectID, targetProjectNamespacesWithReleaseNamespace)
	valuesContentBytes, err := values.ToYAML()
	if err != nil {
		projectHelmChartStatus.ProjectHelmChartStatus = "Error"
		projectHelmChartStatus.ProjectHelmChartStatusMessage = "Could not compute valid values.yaml for this HelmChart from the provided spec.values."
		return nil, projectHelmChartStatus, fmt.Errorf("unable to marshall spec.values of %s/%s: %s", projectHelmChart.Namespace, projectHelmChart.Name, err)
	}

	projectHelmChartRoleBindings, err := h.getRoleBindings(projectID, targetProjectNamespaces, projectHelmChart)
	if err != nil {
		projectHelmChartStatus.ProjectHelmChartStatus = "Error"
		projectHelmChartStatus.ProjectHelmChartStatusMessage = "Could not construct RoleBindings to provide permissions for the ProjectHelmChart"
		return nil, projectHelmChartStatus, fmt.Errorf("unable to calculate rolebindings for ProjectHelmChart %s/%s: %s", projectHelmChart.Namespace, projectHelmChart.Name, err)
	}

	var objs []runtime.Object
	if projectReleaseNamespace != nil {
		// add only if necessary
		objs = []runtime.Object{projectReleaseNamespace}
	}
	objs = append(objs, projectHelmChartRoleBindings...)
	objs = append(objs,
		h.getHelmChart(string(valuesContentBytes), projectHelmChart),
		h.getHelmRelease(projectHelmChart),
	)

	projectHelmChartStatus.DashboardValues, err = h.getStatusFromConfigmaps(projectHelmChart)
	if err != nil {
		projectHelmChartStatus.ProjectHelmChartStatus = "StatusError"
		projectHelmChartStatus.ProjectHelmChartStatusMessage = "Could not retrieve status.dashboardValues from deployed ConfigMaps"
		// still create the objects but leave the ProjectHelmChart in a StatusError if we can't get the values
		return objs, projectHelmChartStatus, err
	}

	projectHelmChartStatus.ProjectHelmChartStatus = "Validated"
	projectHelmChartStatus.ProjectHelmChartStatusMessage = "ProjectHelmChart is valid. HelmChart and HelmRelease should be deployed."
	return objs, projectHelmChartStatus, nil
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

func (h *handler) getRoleBindings(projectID string, targetProjectNamespaces []string, projectHelmChart *v1alpha1.ProjectHelmChart) ([]runtime.Object, error) {
	var rolebindings []runtime.Object
	for _, k8sRole := range common.DefaultK8sRoles {
		subjects := h.subjectRoleGetter.GetSubjects(targetProjectNamespaces, k8sRole)
		if subjects == nil {
			// nothing to add
			continue
		}
		releaseNamespace, releaseName := h.getReleaseNamespaceAndName(projectHelmChart)
		roles, err := h.getProjectHelmChartRoles(k8sRole, projectHelmChart)
		if err != nil {
			return nil, err
		}
		for _, role := range roles {
			rolebindings = append(rolebindings, &rbac.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-%s", releaseName, k8sRole),
					Namespace: releaseNamespace,
					Labels: map[string]string{
						common.HelmProjectOperatedLabel:        "true",
						common.HelmProjectOperatorProjectLabel: projectID,
					},
				},
				Subjects: subjects,
				RoleRef: rbac.RoleRef{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "Role",
					Name:     role.Name,
				},
			})
		}
	}
	return rolebindings, nil
}

func (h *handler) getProjectHelmChartRoles(k8sRole string, projectHelmChart *v1alpha1.ProjectHelmChart) ([]rbac.Role, error) {
	releaseNamespace, releaseName := h.getReleaseNamespaceAndName(projectHelmChart)
	roleList, err := h.roles.List(releaseNamespace, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s,%s=%s",
			common.HelmProjectOperatorProjectHelmChartRoleLabel, releaseName,
			common.HelmProjectOperatorProjectHelmChartRoleAggregateFromLabel, k8sRole),
	})
	if err != nil {
		return nil, err
	}
	if roleList == nil {
		return nil, nil
	}
	return roleList.Items, nil
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
		common.HelmProjectOperatedLabel:               "true",
		common.HelmProjectOperatorHelmApiVersionLabel: strings.SplitN(projectHelmChart.Spec.HelmApiVersion, "/", 2)[0],
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
		return h.systemNamespace, fmt.Sprintf("%s-%s", projectHelmChart.Name, h.opts.ReleaseName)
	}
	if len(h.opts.SystemProjectLabelValue) == 0 {
		// ProjectLabel exists, which means that we are creating ProjectHelmCharts in different namespaces that HelmCharts or HelmReleases
		// However, project release namespaces are not created, so all Helm chart deployments will be in the system namespace
		// Solution: use projectHelmChart namespace as the differentiator per release
		return projectHelmChart.Namespace, fmt.Sprintf("%s-%s", projectHelmChart.Name, h.opts.ReleaseName)
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

func (h *handler) getValues(projectHelmChart *v1alpha1.ProjectHelmChart, projectID string, targetProjectNamespaces []string) v1alpha1.GenericMap {
	// default values that are set if the user does not provide them
	values := map[string]interface{}{
		"global": map[string]interface{}{
			"cattle": map[string]interface{}{
				"systemDefaultRegistry": h.opts.SystemDefaultRegistry,
				"url":                   h.opts.CattleURL,
			},
		},
	}

	// overlay provided values, which will override the above values if provided
	values = data.MergeMaps(values, projectHelmChart.Spec.Values)

	// required project-basd values that must be set even if user tries to override them
	requiredOverrides := map[string]interface{}{
		"global": map[string]interface{}{
			"cattle": map[string]interface{}{
				"clusterId":                h.opts.ClusterID,
				"projectNamespaces":        targetProjectNamespaces,
				"projectID":                projectID,
				"systemProjectID":          h.opts.SystemProjectLabelValue,
				"projectNamespaceSelector": h.getProjectNamespaceSelector(projectHelmChart, projectID),
			},
		},
	}
	// overlay required values, which will override the above values even if provided
	values = data.MergeMaps(values, requiredOverrides)

	return values
}

func (h *handler) getStatusFromConfigmaps(projectHelmChart *v1alpha1.ProjectHelmChart) (v1alpha1.GenericMap, error) {
	releaseNamespace, releaseName := h.getReleaseNamespaceAndName(projectHelmChart)
	configMapList, err := h.configmaps.List(releaseNamespace, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", common.HelmProjectOperatorDashboardValuesConfigMapLabel, releaseName),
	})
	if err != nil {
		return nil, err
	}
	if configMapList == nil {
		return nil, nil
	}
	var values v1alpha1.GenericMap
	for _, configMap := range configMapList.Items {
		for jsonKey, jsonContent := range configMap.Data {
			if !strings.HasSuffix(jsonKey, ".json") {
				logrus.Errorf("dashboard values configmap %s/%s has non-JSON key %s, expected only keys ending with .json. skipping...", configMap.Namespace, configMap.Name, jsonKey)
				continue
			}
			var jsonMap map[string]interface{}
			err := json.Unmarshal([]byte(jsonContent), &jsonMap)
			if err != nil {
				logrus.Errorf("could not marshall content in dashboard values configmap %s/%s in key %s (err='%s'). skipping...", configMap.Namespace, configMap.Name, jsonKey, err)
				continue
			}
			values = data.MergeMapsConcatSlice(values, jsonMap)
		}
	}
	return values, nil
}
