package common

import (
	rbac "k8s.io/api/rbac/v1"
)

func GetDefaultClusterRoles(opts Options) map[string]string {
	clusterRoles := make(map[string]string)
	if len(opts.AdminClusterRole) > 0 {
		clusterRoles["admin"] = opts.AdminClusterRole
	}
	if len(opts.EditClusterRole) > 0 {
		clusterRoles["edit"] = opts.EditClusterRole
	}
	if len(opts.ViewClusterRole) > 0 {
		clusterRoles["view"] = opts.ViewClusterRole
	}
	return clusterRoles
}

func IsDefaultClusterRoleRef(opts Options, roleRefName string) (string, bool) {
	for subjectRole, defaultClusterRoleName := range GetDefaultClusterRoles(opts) {
		if roleRefName == defaultClusterRoleName {
			return subjectRole, true
		}
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
