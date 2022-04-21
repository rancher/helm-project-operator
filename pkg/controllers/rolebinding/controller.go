package rolebinding

import (
	"context"

	"github.com/aiyengar2/helm-project-operator/pkg/controllers/common"
	corecontrollers "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	rbacv1 "github.com/rancher/wrangler/pkg/generated/controllers/rbac/v1"
	rbac "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type handler struct {
	rolebindings        rbacv1.RoleBindingController
	clusterrolebindings rbacv1.ClusterRoleBindingController
	namespaces          corecontrollers.NamespaceController
	namespaceCache      corecontrollers.NamespaceCache
	tracker             SubjectRoleTracker
}

func Register(
	ctx context.Context,
	rolebindings rbacv1.RoleBindingController,
	clusterrolebindings rbacv1.ClusterRoleBindingController,
	namespaces corecontrollers.NamespaceController,
	namespaceCache corecontrollers.NamespaceCache,
) SubjectRoleGetter {

	tracker := NewSubjectRoleTracker()

	h := &handler{
		rolebindings:        rolebindings,
		clusterrolebindings: clusterrolebindings,
		namespaces:          namespaces,
		namespaceCache:      namespaceCache,
		tracker:             tracker,
	}

	rolebindings.OnChange(ctx, "on-rolebinding-change", h.OnRoleBindingChange)
	rolebindings.OnRemove(ctx, "on-rolebinding-remove", h.OnRoleBindingChange)

	clusterrolebindings.OnChange(ctx, "on-clusterrolebinding-change", h.OnClusterRoleBindingChange)
	clusterrolebindings.OnRemove(ctx, "on-clusterrolebinding-remove", h.OnClusterRoleBindingChange)

	return tracker
}

func (h *handler) OnClusterRoleBindingChange(key string, crb *rbac.ClusterRoleBinding) (*rbac.ClusterRoleBinding, error) {
	if crb == nil {
		return crb, nil
	}
	// check if it is tied to a default k8s role
	k8sRole, ok := common.IsK8sDefaultClusterRoleRef(crb.RoleRef)
	if !ok {
		return crb, nil
	}
	// track subjects
	for _, subject := range crb.Subjects {
		h.tracker.Set(subject, "", k8sRole, crb.DeletionTimestamp == nil)
	}
	// enqueue all namespaces
	namespaces, err := h.namespaceCache.List(labels.Everything())
	if err != nil {
		return crb, err
	}
	for _, namespace := range namespaces {
		if namespace == nil {
			continue
		}
		h.namespaces.Enqueue(namespace.Name)
	}
	return crb, nil
}

func (h *handler) OnRoleBindingChange(key string, rb *rbac.RoleBinding) (*rbac.RoleBinding, error) {
	if rb == nil {
		return rb, nil
	}
	// check if it is tied to a default k8s role
	k8sRole, ok := common.IsK8sDefaultClusterRoleRef(rb.RoleRef)
	if !ok {
		return rb, nil
	}
	// track subjects
	for _, subject := range rb.Subjects {
		h.tracker.Set(subject, rb.Namespace, k8sRole, rb.DeletionTimestamp == nil)
	}
	// enqueue namespace
	h.namespaces.Enqueue(rb.Namespace)
	return rb, nil
}
