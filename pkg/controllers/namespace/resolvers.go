package namespace

import (
	"context"

	"github.com/rancher/wrangler/pkg/relatedresource"
	"k8s.io/apimachinery/pkg/runtime"
)

func (h *handler) initResolvers(ctx context.Context) {
	relatedresource.WatchClusterScoped(ctx, "watch-project-registration-data", h.resolveConfigMapToNamespace, h.namespaces, h.configmaps)
}

func (h *handler) resolveConfigMapToNamespace(namespace, name string, obj runtime.Object) ([]relatedresource.Key, error) {
	// enqueue based on configMap name match
	if name != h.getConfigMapName() {
		return nil, nil
	}
	return []relatedresource.Key{{
		Name: namespace,
	}}, nil
}
