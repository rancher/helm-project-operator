package project

import (
	"fmt"

	"github.com/aiyengar2/helm-project-operator/pkg/apis/helm.cattle.io/v1alpha1"
	"github.com/aiyengar2/helm-project-operator/pkg/controllers/common"
	rbac "k8s.io/api/rbac/v1"
)

// Note: each resource created here should have a resolver set in resolvers.go

func (h *handler) getK8sRoleToSubjectsFromRoleBindings(projectHelmChart *v1alpha1.ProjectHelmChart) (map[string][]rbac.Subject, error) {
	k8sRoleToSubjectMap := make(map[string]map[string]rbac.Subject)
	for _, k8sRole := range common.DefaultK8sRoles {
		k8sRoleToSubjectMap[k8sRole] = make(map[string]rbac.Subject)
	}
	defaultOperatorRoleBindings, err := h.rolebindingCache.GetByIndex(RoleBindingInRegistrationNamespaceByReleaseName, RoleBindingReferencesDefaultOperatorRole)
	if err != nil {
		return nil, err
	}
	for _, rb := range defaultOperatorRoleBindings {
		if rb == nil {
			continue
		}
		k8sRole, isDefaultRoleRef := common.GetK8sRoleFromOperatorDefaultRoleName(h.opts.ReleaseName, rb.RoleRef.Name)
		if !isDefaultRoleRef {
			continue
		}
		filteredSubjects := common.FilterToUsersAndGroups(rb.Subjects)
		currSubjects := k8sRoleToSubjectMap[k8sRole]
		for _, filteredSubject := range filteredSubjects {
			// collect into a map to avoid putting duplicates of the same subject
			// we use an index of kind and name since a Group can have the same name as a User, but should be considered separate
			currSubjects[fmt.Sprintf("%s-%s", filteredSubject.Kind, filteredSubject.Name)] = filteredSubject
		}
	}
	// convert back into list so that no duplicates are created
	k8sRoleToSubjects := make(map[string][]rbac.Subject)
	for _, k8sRole := range common.DefaultK8sRoles {
		subjects := []rbac.Subject{}
		for _, subject := range k8sRoleToSubjectMap[k8sRole] {
			subjects = append(subjects, subject)
		}
		k8sRoleToSubjects[k8sRole] = subjects
	}
	return k8sRoleToSubjects, nil
}
