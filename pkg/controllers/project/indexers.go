package project

import (
	"fmt"

	v1alpha1 "github.com/rancher/helm-project-operator/pkg/apis/helm.cattle.io/v1alpha1"
	"github.com/rancher/helm-project-operator/pkg/controllers/common"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
)

const (
	// All namespaces
	ProjectHelmChartByReleaseName = "helm.cattle.io/project-helm-chart-by-release-name"

	// Registration namespaces only
	RoleBindingInRegistrationNamespaceByRoleRef = "helm.cattle.io/role-binding-in-registration-ns-by-role-ref"
	ClusterRoleBindingByRoleRef                 = "helm.cattle.io/cluster-role-binding-by-role-ref"
	BindingReferencesDefaultOperatorRole        = "bound-to-default-role"

	// Release namespaces only
	RoleInReleaseNamespaceByReleaseNamespaceName      = "helm.cattle.io/role-in-release-ns-by-release-namespace-name"
	ConfigMapInReleaseNamespaceByReleaseNamespaceName = "helm.cattle.io/configmap-in-release-ns-by-release-namespace-name"
)

// NamespacedBindingReferencesDefaultOperatorRole is the index used to mark a RoleBinding as one that targets
// one of the default operator roles (supplied in RuntimeOptions under AdminClusterRole, EditClusterRole, and ViewClusterRole)
func NamespacedBindingReferencesDefaultOperatorRole(namespace string) string {
	return fmt.Sprintf("%s/%s", namespace, BindingReferencesDefaultOperatorRole)
}

// initIndexers initializes indexers that allow for more efficient computations on related resources without relying on additional
// calls to be made to the Kubernetes API by referencing the cache instead
func (h *handler) initIndexers() {
	h.projectHelmChartCache.AddIndexer(ProjectHelmChartByReleaseName, h.projectHelmChartToReleaseName)

	h.rolebindingCache.AddIndexer(RoleBindingInRegistrationNamespaceByRoleRef, h.roleBindingInRegistrationNamespaceToRoleRef)

	h.clusterrolebindingCache.AddIndexer(ClusterRoleBindingByRoleRef, h.clusterRoleBindingToRoleRef)

	h.roleCache.AddIndexer(RoleInReleaseNamespaceByReleaseNamespaceName, h.roleInReleaseNamespaceToReleaseNamespaceName)

	h.configmapCache.AddIndexer(ConfigMapInReleaseNamespaceByReleaseNamespaceName, h.configMapInReleaseNamespaceToReleaseNamespaceName)
}

func (h *handler) projectHelmChartToReleaseName(projectHelmChart *v1alpha1.ProjectHelmChart) ([]string, error) {
	if projectHelmChart == nil {
		return nil, nil
	}
	_, releaseName := h.getReleaseNamespaceAndName(projectHelmChart)
	return []string{releaseName}, nil
}

func (h *handler) roleBindingInRegistrationNamespaceToRoleRef(rb *rbacv1.RoleBinding) ([]string, error) {
	if rb == nil {
		return nil, nil
	}
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

func (h *handler) clusterRoleBindingToRoleRef(crb *rbacv1.ClusterRoleBinding) ([]string, error) {
	if crb == nil {
		return nil, nil
	}
	_, isDefaultRoleRef := common.IsDefaultClusterRoleRef(h.opts, crb.RoleRef.Name)
	if !isDefaultRoleRef {
		// we only care about rolebindings in the registration namespace that are tied to the default roles
		// created by this operator
		return nil, nil
	}
	// keep track of this rolebinding in the index so we can grab it later
	return []string{BindingReferencesDefaultOperatorRole}, nil
}

func (h *handler) roleInReleaseNamespaceToReleaseNamespaceName(role *rbacv1.Role) ([]string, error) {
	if role == nil {
		return nil, nil
	}
	return h.getReleaseIndexFromNamespaceAndLabels(role.Namespace, role.Labels, common.HelmProjectOperatorProjectHelmChartRoleLabel)
}

func (h *handler) configMapInReleaseNamespaceToReleaseNamespaceName(configmap *corev1.ConfigMap) ([]string, error) {
	if configmap == nil {
		return nil, nil
	}
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

	return []string{fmt.Sprintf("%s/%s", namespace, releaseName)}, nil
}
