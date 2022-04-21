package common

import (
	rbac "k8s.io/api/rbac/v1"
)

var (
	ClusterAdminClusterRoleRef = rbac.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "ClusterRole",
		Name:     "cluster-admin",
	}
	AdminClusterRoleRef = rbac.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "ClusterRole",
		Name:     "admin",
	}
	EditClusterRoleRef = rbac.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "ClusterRole",
		Name:     "edit",
	}
	ViewClusterRoleRef = rbac.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "ClusterRole",
		Name:     "view",
	}

	DefaultK8sRoles = []string{
		ClusterAdminClusterRoleRef.Name,
		AdminClusterRoleRef.Name,
		EditClusterRoleRef.Name,
		ViewClusterRoleRef.Name,
	}
)

func IsK8sDefaultClusterRoleRef(roleRef rbac.RoleRef) (string, bool) {
	switch roleRef {
	case ClusterAdminClusterRoleRef, AdminClusterRoleRef, EditClusterRoleRef, ViewClusterRoleRef:
		return roleRef.Name, true
	default:
		return "", false
	}
}
