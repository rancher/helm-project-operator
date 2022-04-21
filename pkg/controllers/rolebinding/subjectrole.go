package rolebinding

import "github.com/aiyengar2/helm-project-operator/pkg/controllers/common"

type subjectRole struct {
	ClusterAdmin bool
	Admin        bool
	Edit         bool
	View         bool
}

func (r subjectRole) Set(k8sRole string, hasRole bool) subjectRole {
	switch k8sRole {
	case common.ClusterAdminClusterRoleRef.Name:
		r.ClusterAdmin = hasRole
	case common.AdminClusterRoleRef.Name:
		r.Admin = hasRole
	case common.EditClusterRoleRef.Name:
		r.Edit = hasRole
	case common.ViewClusterRoleRef.Name:
		r.View = hasRole
	}
	return r
}

func (r subjectRole) Has(k8sRole string) bool {
	switch k8sRole {
	case common.ClusterAdminClusterRoleRef.Name:
		return r.ClusterAdmin
	case common.AdminClusterRoleRef.Name:
		return r.Admin
	case common.EditClusterRoleRef.Name:
		return r.Edit
	case common.ViewClusterRoleRef.Name:
		return r.View
	}
	return false
}

func (r subjectRole) HasNoRole() bool {
	return !r.ClusterAdmin && !r.Admin && !r.Edit && !r.View
}
