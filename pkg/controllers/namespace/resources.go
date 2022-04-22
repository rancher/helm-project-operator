package namespace

import (
	"fmt"
	"strings"

	"github.com/aiyengar2/helm-project-operator/pkg/apis/helm.cattle.io"
	"github.com/aiyengar2/helm-project-operator/pkg/apis/helm.cattle.io/v1alpha1"
	"github.com/aiyengar2/helm-project-operator/pkg/controllers/common"
	v1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Note: each resource created here should have a resolver set in resolvers.go
// The only exception is namespaces since those are handled by the main controller OnChange and OnRemove

func (h *handler) getProjectRegistrationNamespace(projectID string, isOrphaned bool, namespace *v1.Namespace) *v1.Namespace {
	return &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:        fmt.Sprintf(common.ProjectRegistrationNamespaceFmt, projectID),
			Annotations: common.GetProjectNamespaceAnnotations(projectID, h.opts.ProjectLabel, h.opts.ClusterID),
			Labels:      common.GetProjectNamespaceLabels(projectID, h.opts.ProjectLabel, projectID, isOrphaned),
		},
	}
}

func (h *handler) getConfigMap(projectID string, namespace *v1.Namespace) *v1.ConfigMap {
	return &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      h.getConfigMapName(),
			Namespace: namespace.Name,
			Labels:    common.GetCommonLabels(projectID),
		},
		Data: map[string]string{
			"values.yaml":    h.valuesYaml,
			"questions.yaml": h.questionsYaml,
		},
	}
}

func (h *handler) getRoles(projectID string, namespace *v1.Namespace) []runtime.Object {
	var objs []runtime.Object

	for _, k8sRole := range common.DefaultK8sRoles {
		objs = append(objs, &rbac.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:      h.getRoleName(k8sRole),
				Namespace: namespace.Name,
				Labels:    common.GetCommonLabels(projectID),
			},
			Rules: []rbac.PolicyRule{{
				APIGroups: []string{helm.GroupName},
				Resources: []string{v1alpha1.ProjectHelmChartResourceName},
				// What is this verb?
				//
				// This verb doesn't actually provide any permissions, but it makes it such that
				// a user would not be able to create a RoleBinding to this role without having this
				// permission as they would be blocked by Kubernetes's escalation checks from granting
				// permissions to a Role that they don't have permissions for in the first place
				//
				// To gain initial permissions to this role, it's expected that RoleBindings will be created
				// by this controller. See h.getRoleBindings
				Verbs: []string{fmt.Sprintf("grant-%s", k8sRole)},
			}},
		})
	}

	return objs
}

func (h *handler) getRoleBindings(projectID string, projectNamespaces []*v1.Namespace, namespace *v1.Namespace) []runtime.Object {
	var objs []runtime.Object

	var targetNamespaces []string
	for _, namespace := range projectNamespaces {
		if namespace == nil {
			continue
		}
		targetNamespaces = append(targetNamespaces, namespace.Name)
	}

	for _, k8sRole := range common.DefaultK8sRoles {
		objs = append(objs, &rbac.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      h.getRoleBindingName(k8sRole),
				Namespace: namespace.Name,
				Labels:    common.GetCommonLabels(projectID),
			},
			// Subjects for RoleBinding is determined by seeing who has the k8sRole
			// across all namespaces in a project. If they are missing the default k8s
			// role in even one namespace, they will not receive this permission automatically
			Subjects: common.FilterToUsersAndGroups(
				h.subjectRoleGetter.GetSubjects(targetNamespaces, k8sRole),
			),
			RoleRef: rbac.RoleRef{
				APIGroup: rbac.GroupName,
				Kind:     "Role",
				Name:     h.getRoleName(k8sRole),
			},
		})
	}

	return objs
}

func (h *handler) getConfigMapName() string {
	return strings.ReplaceAll(h.opts.HelmApiVersion, "/", ".")
}

func (h *handler) getRoleName(k8sRole string) string {
	return common.GetOperatorDefaultRoleName(h.opts.ReleaseName, k8sRole)
}

func (h *handler) getRoleBindingName(k8sRole string) string {
	return common.GetOperatorDefaultRoleName(h.opts.ReleaseName, k8sRole)
}
