package project

import (
	"fmt"

	"github.com/aiyengar2/helm-project-operator/pkg/apis/helm.cattle.io/v1alpha1"
	"github.com/aiyengar2/helm-project-operator/pkg/controllers/common"
	corev1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
)

const (
	// All namespaces
	ProjectHelmChartByReleaseName = "helm.cattle.io/project-helm-chart-by-release-name"

	// Registration namespaces only
	RoleBindingInRegistrationNamespaceByRoleRef = "helm.cattle.io/role-binding-in-registration-ns-by-role-ref"
	ClusterRoleBindingByRoleRef                 = "helm.cattle.io/cluster-role-binding-by-role-ref"
	BindingReferencesDefaultOperatorRole        = "bound-to-default-role"

	// Release namespaces only
	RoleInReleaseNamespaceByReleaseName      = "helm.cattle.io/role-in-release-ns-by-release-name"
	ConfigMapInReleaseNamespaceByReleaseName = "helm.cattle.io/configmap-in-release-ns-by-release-name"
)

func NamespacedBindingReferencesDefaultOperatorRole(namespace string) string {
	return fmt.Sprintf("%s/%s", namespace, BindingReferencesDefaultOperatorRole)
}

func (h *handler) initIndexers() {
	h.projectHelmChartCache.AddIndexer(ProjectHelmChartByReleaseName, h.projectHelmChartToReleaseName)

	h.rolebindingCache.AddIndexer(RoleBindingInRegistrationNamespaceByRoleRef, h.roleBindingInRegistrationNamespaceToRoleRef)

	h.clusterrolebindingCache.AddIndexer(ClusterRoleBindingByRoleRef, h.clusterRoleBindingToRoleRef)

	h.roleCache.AddIndexer(RoleInReleaseNamespaceByReleaseName, h.roleInReleaseNamespaceToReleaseName)

	h.configmapCache.AddIndexer(ConfigMapInReleaseNamespaceByReleaseName, h.configMapInReleaseNamespaceToReleaseName)
}

func (h *handler) projectHelmChartToReleaseName(projectHelmChart *v1alpha1.ProjectHelmChart) ([]string, error) {
	_, releaseName := h.getReleaseNamespaceAndName(projectHelmChart)
	return []string{releaseName}, nil
}

func (h *handler) roleBindingInRegistrationNamespaceToRoleRef(rb *rbac.RoleBinding) ([]string, error) {
	isProjectRegistrationNamespace, err := h.projectGetter.IsProjectRegistrationNamespace(rb.Namespace)
	if err != nil {
		return nil, err
	}
	if !isProjectRegistrationNamespace {
		return nil, nil
	}
	_, isDefaultRoleRef := common.IsDefaultClusterRoleRef(h.opts, rb.RoleRef.Name)
	if !isDefaultRoleRef {
		// we only care about rolebindings in the registration namespace that are tied to the default roles
		// created by this operator
		return nil, nil
	}
	// keep track of this rolebinding in the index so we can grab it later
	return []string{NamespacedBindingReferencesDefaultOperatorRole(rb.Namespace)}, nil
}

func (h *handler) clusterRoleBindingToRoleRef(crb *rbac.ClusterRoleBinding) ([]string, error) {
	_, isDefaultRoleRef := common.IsDefaultClusterRoleRef(h.opts, crb.RoleRef.Name)
	if !isDefaultRoleRef {
		// we only care about rolebindings in the registration namespace that are tied to the default roles
		// created by this operator
		return nil, nil
	}
	// keep track of this rolebinding in the index so we can grab it later
	return []string{BindingReferencesDefaultOperatorRole}, nil
}

func (h *handler) roleInReleaseNamespaceToReleaseName(role *rbac.Role) ([]string, error) {
	return h.getReleaseIndexFromNamespaceAndLabels(role.Namespace, role.Labels, common.HelmProjectOperatorProjectHelmChartRoleLabel)
}

func (h *handler) configMapInReleaseNamespaceToReleaseName(configmap *corev1.ConfigMap) ([]string, error) {
	return h.getReleaseIndexFromNamespaceAndLabels(configmap.Namespace, configmap.Labels, common.HelmProjectOperatorDashboardValuesConfigMapLabel)
}

func (h *handler) getReleaseIndexFromNamespaceAndLabels(namespace string, labels map[string]string, releaseLabel string) ([]string, error) {
	if labels == nil {
		return nil, nil
	}
	releaseName, ok := labels[releaseLabel]
	if !ok {
		return nil, nil
	}

	// note: just checking the release label here is not sufficient since it's possible that someone could
	// create a configMap or role in a non-release namespace and tie it to this index if so. We need to
	// grab all ProjectHelmCharts so we can go from release-name -> release-namespace and verify that the
	// object marked with this label also exists in the correct release namespace for a ProjectHelmChart
	// tied to this release

	// grab all projectHelmChart tied to this release
	projectHelmCharts, err := h.projectHelmChartCache.GetByIndex(ProjectHelmChartByReleaseName, releaseName)
	if err != nil {
		return nil, err
	}
	if len(projectHelmCharts) == 0 {
		// release name is invalid, it doesn't correspond to a project helm chart
		return nil, nil
	}
	for _, projectHelmChart := range projectHelmCharts {
		if projectHelmChart == nil {
			continue
		}
		releaseNamespace, _ := h.getReleaseNamespaceAndName(projectHelmChart)
		if releaseNamespace == namespace {
			// key on object both matches an existing release and is in the namespace of that given release
			// therefore, we can tie this object to this release
			return []string{releaseName}, nil
		}
	}
	return nil, nil
}
