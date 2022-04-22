package common

import (
	"fmt"
	"strings"

	rbac "k8s.io/api/rbac/v1"
)

var (
	ClusterAdminClusterRoleRef = rbac.RoleRef{
		APIGroup: rbac.GroupName,
		Kind:     "ClusterRole",
		Name:     "cluster-admin",
	}
	AdminClusterRoleRef = rbac.RoleRef{
		APIGroup: rbac.GroupName,
		Kind:     "ClusterRole",
		Name:     "admin",
	}
	EditClusterRoleRef = rbac.RoleRef{
		APIGroup: rbac.GroupName,
		Kind:     "ClusterRole",
		Name:     "edit",
	}
	ViewClusterRoleRef = rbac.RoleRef{
		APIGroup: rbac.GroupName,
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

func GetOperatorDefaultRolePrefix(releaseName string) string {
	return fmt.Sprintf("hpo-%s-", releaseName)
}

func GetOperatorDefaultRoleName(releaseName, k8sRole string) string {
	return GetOperatorDefaultRolePrefix(releaseName) + k8sRole
}

func GetK8sRoleFromOperatorDefaultRoleName(releaseName, roleName string) (string, bool) {
	prefix := GetOperatorDefaultRolePrefix(releaseName)
	if !strings.HasPrefix(roleName, prefix) {
		return "", false
	}
	trimmed := strings.TrimPrefix(roleName, prefix)
	switch trimmed {
	case ClusterAdminClusterRoleRef.Name, AdminClusterRoleRef.Name, EditClusterRoleRef.Name, ViewClusterRoleRef.Name:
		return trimmed, true
	}
	return "", false
}

func FilterToUsersAndGroups(subjects []rbac.Subject) []rbac.Subject {
	var filtered []rbac.Subject
	for _, subject := range subjects {
		if subject.APIGroup != rbac.GroupName {
			continue
		}
		if subject.Kind != rbac.UserKind && subject.Kind != rbac.GroupKind {
			// we do not automatically bind service accounts, only users and groups
			continue
		}
		// note: we are purposefully omitting namespace here since it is not necessary even if set
		filtered = append(filtered, rbac.Subject{
			APIGroup: subject.APIGroup,
			Kind:     subject.Kind,
			Name:     subject.Name,
		})
	}
	return filtered
}
