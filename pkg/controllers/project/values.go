package project

import (
	v1alpha1 "github.com/aiyengar2/helm-project-operator/pkg/apis/helm.cattle.io/v1alpha1"
	"github.com/rancher/wrangler/pkg/data"
)

// getValues returns the values.yaml that should be applied for this ProjectHelmChart after processing default and required overrides
func (h *handler) getValues(projectHelmChart *v1alpha1.ProjectHelmChart, projectID string, targetProjectNamespaces []string) v1alpha1.GenericMap {
	// default values that are set if the user does not provide them
	values := map[string]interface{}{
		"global": map[string]interface{}{
			"cattle": map[string]interface{}{
				"systemDefaultRegistry": h.opts.SystemDefaultRegistry,
				"url":                   h.opts.CattleURL,
			},
		},
	}

	// overlay provided values, which will override the above values if provided
	values = data.MergeMaps(values, projectHelmChart.Spec.Values)

	// required project-based values that must be set even if user tries to override them
	requiredOverrides := map[string]interface{}{
		"global": map[string]interface{}{
			"cattle": map[string]interface{}{
				"clusterId":                h.opts.ClusterID,
				"projectNamespaces":        targetProjectNamespaces,
				"projectID":                projectID,
				"systemProjectID":          h.opts.SystemProjectLabelValue,
				"projectNamespaceSelector": h.getProjectNamespaceSelector(projectHelmChart, projectID),
			},
		},
	}
	// overlay required values, which will override the above values even if provided
	values = data.MergeMaps(values, requiredOverrides)

	return values
}
