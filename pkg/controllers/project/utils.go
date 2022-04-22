package project

import (
	"fmt"
	"strings"

	"github.com/aiyengar2/helm-project-operator/pkg/apis/helm.cattle.io/v1alpha1"
	"github.com/aiyengar2/helm-project-operator/pkg/controllers/common"
)

func (h *handler) getProjectID(projectHelmChart *v1alpha1.ProjectHelmChart) (string, error) {
	if len(h.opts.ProjectLabel) == 0 {
		return "", nil
	}
	projectRegistrationNamespace, err := h.namespaceCache.Get(projectHelmChart.Namespace)
	if err != nil {
		return "", fmt.Errorf("unable to parse projectID for projectHelmChart %s/%s: %s", projectHelmChart.Namespace, projectHelmChart.Name, err)
	}
	projectID, ok := projectRegistrationNamespace.Labels[h.opts.ProjectLabel]
	if !ok {
		return "", nil
	}
	return projectID, nil
}

func (h *handler) getProjectNamespaceSelector(projectHelmChart *v1alpha1.ProjectHelmChart, projectID string) map[string]interface{} {
	if len(h.opts.ProjectLabel) == 0 {
		// Use the projectHelmChart selector as the namespaceSelector
		if projectHelmChart.Spec.ProjectNamespaceSelector == nil {
			return map[string]interface{}{}
		}
		return map[string]interface{}{
			"matchLabels":      projectHelmChart.Spec.ProjectNamespaceSelector.MatchLabels,
			"matchExpressions": projectHelmChart.Spec.ProjectNamespaceSelector.MatchExpressions,
		}
	}
	if len(h.opts.SystemProjectLabelValue) == 0 {
		// Release namespace is not created, so use namespaceSelector provided tied to projectID
		return map[string]interface{}{
			"matchLabels": map[string]string{
				h.opts.ProjectLabel: projectID,
			},
		}
	}
	// use the HelmProjectOperated label
	return map[string]interface{}{
		"matchLabels": map[string]string{
			common.HelmProjectOperatorProjectLabel: projectID,
		},
	}
}

func (h *handler) getReleaseNamespaceAndName(projectHelmChart *v1alpha1.ProjectHelmChart) (string, string) {
	projectReleaseName := fmt.Sprintf("%s-%s", projectHelmChart.Name, h.opts.ReleaseName)
	if h.opts.Singleton {
		// This changes the naming scheme of the deployed resources such that only one can every be created per namespace
		projectReleaseName = fmt.Sprintf("%s-%s", projectHelmChart.Namespace, h.opts.ReleaseName)
	}
	if len(h.opts.ProjectLabel) == 0 {
		// Underlying Helm releases will all be created in the same namespace
		return h.systemNamespace, projectReleaseName
	}
	if len(h.opts.SystemProjectLabelValue) == 0 {
		// Underlying Helm releases will be created in the project registration namespace where the ProjectHelmChart is registered
		return projectHelmChart.Namespace, projectReleaseName
	}
	// Underlying Helm releases will be created in dedicated project release namespaces
	return projectReleaseName, projectReleaseName
}

func getLabels(projectHelmChart *v1alpha1.ProjectHelmChart) map[string]string {
	return map[string]string{
		common.HelmProjectOperatedLabel:               "true",
		common.HelmProjectOperatorHelmApiVersionLabel: strings.SplitN(projectHelmChart.Spec.HelmApiVersion, "/", 2)[0],
	}
}
