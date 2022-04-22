package namespace

import (
	"context"

	"github.com/aiyengar2/helm-project-operator/pkg/controllers/common"
	"github.com/rancher/wrangler/pkg/relatedresource"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Note: each resource created in resources.go should have a resolver handler here
// The only exception is namespaces since those are handled by the main controller OnChange and OnRemove

func (h *handler) initResolvers(ctx context.Context) {
	relatedresource.WatchClusterScoped(
		ctx, "watch-project-registration-namespace-data", h.resolveProjectRegistrationNamespaceData, h.namespaces,
		h.configmaps, h.rolebindings, h.roles,
	)
}

func (h *handler) resolveProjectRegistrationNamespaceData(namespace, name string, obj runtime.Object) ([]relatedresource.Key, error) {
	if !h.projectRegistrationNamespaceRegister.Has(namespace) {
		// no longer need to watch for changes to resources in this namespace since it is no longer tracked
		// if the namespace ever becomes unorphaned, we can track it again
		return nil, nil
	}
	if obj == nil {
		return nil, nil
	}
	if configmap, ok := obj.(*corev1.ConfigMap); ok {
		return h.resolveConfigMap(namespace, name, configmap)
	}
	if rb, ok := obj.(*rbacv1.RoleBinding); ok {
		return h.resolveRoleBinding(namespace, name, rb)
	}
	if role, ok := obj.(*rbacv1.Role); ok {
		return h.resolveRole(namespace, name, role)
	}
	return nil, nil
}

func (h *handler) resolveConfigMap(namespace, name string, configmap *corev1.ConfigMap) ([]relatedresource.Key, error) {
	// check if name matches
	if name == h.getConfigMapName() {
		return []relatedresource.Key{{
			Name: namespace,
		}}, nil
	}
	return nil, nil
}

func (h *handler) resolveRoleBinding(namespace, name string, rb *rbacv1.RoleBinding) ([]relatedresource.Key, error) {
	// check if name matches
	for _, k8sRole := range common.DefaultK8sRoles {
		if name == h.getRoleBindingName(k8sRole) {
			return []relatedresource.Key{{
				Name: namespace,
			}}, nil
		}
	}
	return nil, nil
}

func (h *handler) resolveRole(namespace, name string, role *rbacv1.Role) ([]relatedresource.Key, error) {
	// check if name matches
	for _, k8sRole := range common.DefaultK8sRoles {
		if name == h.getRoleName(k8sRole) {
			return []relatedresource.Key{{
				Name: namespace,
			}}, nil
		}
	}
	return nil, nil
}
