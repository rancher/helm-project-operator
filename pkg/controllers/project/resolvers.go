package project

import (
	"context"

	"github.com/aiyengar2/helm-project-operator/pkg/controllers/common"
	"github.com/rancher/wrangler/pkg/apply"
	"github.com/rancher/wrangler/pkg/relatedresource"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
)

func (h *handler) initResolvers(ctx context.Context) {
	relatedresource.Watch(ctx, "sync-helm-resources", h.resolveProjectHelmChartOwned, h.projectHelmCharts, h.helmCharts, h.helmReleases)
	relatedresource.Watch(ctx, "sync-status-configmaps", h.resolveProjectHelmChartStatusChange, h.projectHelmCharts, h.configmaps)
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
