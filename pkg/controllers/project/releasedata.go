package project

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aiyengar2/helm-project-operator/pkg/apis/helm.cattle.io/v1alpha1"
	"github.com/aiyengar2/helm-project-operator/pkg/controllers/common"
	"github.com/rancher/wrangler/pkg/data"
	"github.com/sirupsen/logrus"
	rbac "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Note: each resource created here should have a resolver set in resolvers.go

func (h *handler) getDashboardValuesFromConfigmaps(projectHelmChart *v1alpha1.ProjectHelmChart) (v1alpha1.GenericMap, error) {
	releaseNamespace, releaseName := h.getReleaseNamespaceAndName(projectHelmChart)
	exists, err := h.verifyReleaseNamespaceExists(releaseNamespace)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	configMapList, err := h.configmaps.List(releaseNamespace, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", common.HelmProjectOperatorDashboardValuesConfigMapLabel, releaseName),
	})
	if err != nil {
		return nil, err
	}
	if configMapList == nil {
		return nil, nil
	}
	var values v1alpha1.GenericMap
	for _, configMap := range configMapList.Items {
		for jsonKey, jsonContent := range configMap.Data {
			if !strings.HasSuffix(jsonKey, ".json") {
				logrus.Errorf("dashboard values configmap %s/%s has non-JSON key %s, expected only keys ending with .json. skipping...", configMap.Namespace, configMap.Name, jsonKey)
				continue
			}
			var jsonMap map[string]interface{}
			err := json.Unmarshal([]byte(jsonContent), &jsonMap)
			if err != nil {
				logrus.Errorf("could not marshall content in dashboard values configmap %s/%s in key %s (err='%s'). skipping...", configMap.Namespace, configMap.Name, jsonKey, err)
				continue
			}
			values = data.MergeMapsConcatSlice(values, jsonMap)
		}
	}
	return values, nil
}

func (h *handler) getK8sRoleToRoleRefsFromRoles(projectHelmChart *v1alpha1.ProjectHelmChart) (map[string][]rbac.RoleRef, error) {
	k8sRoleToRoleRefs := make(map[string][]rbac.RoleRef)
	for _, k8sRole := range common.DefaultK8sRoles {
		k8sRoleToRoleRefs[k8sRole] = []rbac.RoleRef{}
	}
	releaseNamespace, releaseName := h.getReleaseNamespaceAndName(projectHelmChart)
	exists, err := h.verifyReleaseNamespaceExists(releaseNamespace)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	roleList, err := h.roles.List(releaseNamespace, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", common.HelmProjectOperatorProjectHelmChartRoleLabel, releaseName),
	})
	if err != nil {
		return nil, err
	}
	if roleList == nil {
		return nil, nil
	}
	for _, role := range roleList.Items {
		k8sRole, ok := role.Labels[common.HelmProjectOperatorProjectHelmChartRoleAggregateFromLabel]
		if !ok {
			// cannot assign roles if this label is not provided
			continue
		}
		roleRefs, ok := k8sRoleToRoleRefs[k8sRole]
		if !ok {
			// label value is invalid since it does not point to default k8s role name
			continue
		}
		k8sRoleToRoleRefs[k8sRole] = append(roleRefs, rbac.RoleRef{
			APIGroup: rbac.GroupName,
			Kind:     "Role",
			Name:     role.Name,
		})
	}
	return k8sRoleToRoleRefs, nil
}

func (h *handler) verifyReleaseNamespaceExists(releaseNamespace string) (bool, error) {
	_, err := h.namespaceCache.Get(releaseNamespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// release namespace has not been created yet
			return false, nil
		}
		return false, err
	}
	return true, nil
}
