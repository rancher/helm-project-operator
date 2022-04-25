package project

import (
	"encoding/json"
	"strings"

	"github.com/aiyengar2/helm-project-operator/pkg/apis/helm.cattle.io/v1alpha1"
	"github.com/aiyengar2/helm-project-operator/pkg/controllers/common"
	"github.com/rancher/wrangler/pkg/data"
	"github.com/sirupsen/logrus"
	rbac "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	configMaps, err := h.configmapCache.GetByIndex(ConfigMapInReleaseNamespaceByReleaseName, releaseName)
	if err != nil {
		return nil, err
	}
	var values v1alpha1.GenericMap
	for _, configMap := range configMaps {
		if configMap == nil {
			continue
		}
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
	for _, k8sRole := range common.GetDefaultClusterRoles(h.opts) {
		k8sRoleToRoleRefs[k8sRole] = []rbac.RoleRef{}
	}
	if len(k8sRoleToRoleRefs) == 0 {
		// no roles were defined to be auto-aggregated
		return k8sRoleToRoleRefs, nil
	}
	releaseNamespace, releaseName := h.getReleaseNamespaceAndName(projectHelmChart)
	exists, err := h.verifyReleaseNamespaceExists(releaseNamespace)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	roles, err := h.roleCache.GetByIndex(RoleInReleaseNamespaceByReleaseName, releaseName)
	if err != nil {
		return nil, err
	}
	for _, role := range roles {
		if role == nil {
			continue
		}
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
