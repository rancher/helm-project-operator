package project

import (
	"context"

	"github.com/aiyengar2/helm-locker/pkg/apis/helm.cattle.io/v1alpha1"
	"github.com/aiyengar2/helm-project-operator/pkg/controllers/common"
	v1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	"github.com/rancher/wrangler/pkg/apply"
	"github.com/rancher/wrangler/pkg/relatedresource"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
)

// Note: each resource created in resources.go, registrationdata.go, or releasedata.go should have a resolver handler here
// The only exception is ProjectHelmCharts since those are handled by the main generating controller

func (h *handler) initResolvers(ctx context.Context) {
	if len(h.opts.ProjectLabel) != 0 && len(h.opts.SystemProjectLabelValue) == 0 {
		// Only trigger watching project release namespace if it is created by the operator
		relatedresource.Watch(
			ctx, "watch-project-release-namespace", h.resolveProjectReleaseNamespace, h.projectHelmCharts,
			h.namespaces,
		)
	}

	relatedresource.Watch(
		ctx, "watch-system-namespace-chart-data", h.resolveSystemNamespaceData, h.projectHelmCharts,
		h.helmCharts, h.helmReleases,
	)

	relatedresource.Watch(
		ctx, "watch-project-registration-chart-data", h.resolveProjectRegistrationNamespaceData, h.projectHelmCharts,
		h.rolebindings,
	)

	relatedresource.Watch(
		ctx, "watch-project-release-chart-data", h.resolveProjectReleaseNamespaceData, h.projectHelmCharts,
		h.rolebindings, h.configmaps, h.roles,
	)
}

// Project Release Namespace

func (h *handler) resolveProjectReleaseNamespace(namespace, name string, obj runtime.Object) ([]relatedresource.Key, error) {
	if obj == nil {
		return nil, nil
	}
	ns, ok := obj.(*corev1.Namespace)
	if !ok {
		return nil, nil
	}
	// since the release namespace will be created and owned by the ProjectHelmChart,
	// we can simply leverage is annotations to identify what we should resolve to.
	// If the release namespace is orphaned, the owner annotation should be removed automatically
	return h.resolveProjectHelmChartOwned(ns.Annotations)
}

// System Namespace Data

func (h *handler) resolveSystemNamespaceData(namespace, name string, obj runtime.Object) ([]relatedresource.Key, error) {
	if namespace != h.systemNamespace {
		return nil, nil
	}
	if obj == nil {
		return nil, nil
	}
	// since the HelmChart and HelmRelease will be created and owned by the ProjectHelmChart,
	// we can simply leverage is annotations to identify what we should resolve to.
	if helmChart, ok := obj.(*v1.HelmChart); ok {
		return h.resolveProjectHelmChartOwned(helmChart.Annotations)
	}
	if helmRelease, ok := obj.(*v1alpha1.HelmRelease); ok {
		return h.resolveProjectHelmChartOwned(helmRelease.Annotations)
	}
	return nil, nil
}

// Project Registration Namespace Data

func (h *handler) resolveProjectRegistrationNamespaceData(namespace, name string, obj runtime.Object) ([]relatedresource.Key, error) {
	isProjectRegistrationNamespace, err := h.projectGetter.IsProjectRegistrationNamespace(namespace)
	if err != nil {
		return nil, err
	}
	if !isProjectRegistrationNamespace {
		return nil, nil
	}
	if obj == nil {
		return nil, nil
	}
	if rb, ok := obj.(*rbacv1.RoleBinding); ok {
		return h.resolveProjectRegistrationNamespaceRoleBinding(namespace, name, rb)
	}
	return nil, nil
}

func (h *handler) resolveProjectRegistrationNamespaceRoleBinding(namespace, name string, rb *rbacv1.RoleBinding) ([]relatedresource.Key, error) {
	// we want to re-enqueue the ProjectHelmChart if the rolebinding's ref points to one of the operator default roles
	_, isDefaultRoleRef := common.GetK8sRoleFromOperatorDefaultRoleName(h.opts.ReleaseName, rb.RoleRef.Name)
	if !isDefaultRoleRef {
		return nil, nil
	}
	// re-enqueue all HelmCharts in this project registration namespace
	projectHelmCharts, err := h.projectHelmChartCache.List(namespace, labels.Everything())
	if err != nil {
		return nil, err
	}
	var keys []relatedresource.Key
	for _, projectHelmChart := range projectHelmCharts {
		if projectHelmChart == nil {
			continue
		}
		keys = append(keys, relatedresource.Key{
			Namespace: namespace,
			Name:      projectHelmChart.Name,
		})
	}
	return keys, nil
}

// Project Release Namespace Data

func (h *handler) resolveProjectReleaseNamespaceData(namespace, name string, obj runtime.Object) ([]relatedresource.Key, error) {
	if obj == nil {
		return nil, nil
	}
	if rb, ok := obj.(*rbacv1.RoleBinding); ok {
		// since the rolebinding will be created and owned by the ProjectHelmChart,
		// we can simply leverage is annotations to identify what we should resolve to.
		return h.resolveProjectHelmChartOwned(rb.Annotations)
	}
	if configmap, ok := obj.(*corev1.ConfigMap); ok {
		return h.resolveByProjectReleaseLabelValue(configmap.Labels, common.HelmProjectOperatorDashboardValuesConfigMapLabel)
	}
	if role, ok := obj.(*rbacv1.Role); ok {
		return h.resolveByProjectReleaseLabelValue(role.Labels, common.HelmProjectOperatorProjectHelmChartRoleLabel)
	}
	return nil, nil
}

// Common

func (h *handler) resolveProjectHelmChartOwned(annotations map[string]string) ([]relatedresource.Key, error) {
	// Q: Why aren't we using relatedresource.OwnerResolver?
	// A: in k8s, you can't set an owner reference across namespaces, which means that when --project-label is provided
	// (where the ProjectHelmChart will be outside the systemNamespace where the HelmCharts and HelmReleases are created),
	// ownerReferences will not be set on the object. However, wrangler annotations will be set since those objects are
	// created via a wrangler apply. Therefore, we leverage those annotations to figure out which ProjectHelmChart to enqueue
	if annotations == nil {
		return nil, nil
	}
	ownerNamespace, ok := annotations[apply.LabelNamespace]
	if !ok {
		return nil, nil
	}
	ownerName, ok := annotations[apply.LabelName]
	if !ok {
		return nil, nil
	}

	return []relatedresource.Key{{
		Namespace: ownerNamespace,
		Name:      ownerName,
	}}, nil
}

func (h *handler) resolveByProjectReleaseLabelValue(labels map[string]string, projectReleaseLabel string) ([]relatedresource.Key, error) {
	if labels == nil {
		return nil, nil
	}
	releaseName, ok := labels[projectReleaseLabel]
	if !ok {
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