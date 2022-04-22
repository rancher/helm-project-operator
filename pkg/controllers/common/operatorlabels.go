package common

import (
	"fmt"
	"strings"
)

// Operator Labels
// Note: These labels are automatically applied by the operator to mark resources that are created for a given ProjectHelmChart and Project Operator

// Common

const (
	// HelmProjectOperatedLabel marks all HelmCharts, HelmReleases, and namespaces created by this operator
	HelmProjectOperatedLabel = "helm.cattle.io/helm-project-operated"

	// HelmProjectOperatorProjectLabel is applied to all namespaces targeted by a project only if SystemProjectLabelValue and
	// ProjectLabel are provided, in which case the release namespace of the HelmChart that is deployed will be auto-generated
	// and imported into the system project; since the value of the provided ProjectLabel will not match the value of the ProjectLabel
	// on the generated namespace, this label needs to be added to create a consistent set of labels for global.cattle.projectNamespaceSelector
	// to be able to target
	HelmProjectOperatorProjectLabel = "helm.cattle.io/projectId"
)

func GetCommonLabels(projectID string) map[string]string {
	labels := map[string]string{
		HelmProjectOperatedLabel: "true",
	}
	if len(projectID) != 0 {
		labels[HelmProjectOperatorProjectLabel] = projectID
	}
	return labels
}

// Project Namespaces

const (
	// HelmProjectOperatedNamespaceOrphanedLabel marks all auto-generated namespaces that no longer have resources tracked
	// by this operator; if a namespace has this label, it is safe to delete
	HelmProjectOperatedNamespaceOrphanedLabel = "helm.cattle.io/helm-project-operator-orphaned"
)

func GetProjectNamespaceLabels(projectID, projectLabel, projectLabelValue string, isOrphaned bool) map[string]string {
	labels := GetCommonLabels(projectID)
	if isOrphaned {
		labels[HelmProjectOperatedNamespaceOrphanedLabel] = "true"
	}
	labels[projectLabel] = projectLabelValue
	return labels
}

func GetProjectNamespaceAnnotations(projectID, projectLabel, clusterID string) map[string]string {
	projectIDWithClusterID := projectID
	if len(clusterID) > 0 {
		projectIDWithClusterID = fmt.Sprintf("%s:%s", clusterID, projectID)
	}
	return map[string]string{
		projectLabel: projectIDWithClusterID,
	}
}

// Helm Resources (HelmCharts and HelmReleases)

const (
	// HelmProjectOperatorHelmApiVersionLabel is a label that identifies the HelmApiVersion that a HelmChart or HelmRelease is tied to
	// This is used to identify whether a HelmChart or HelmRelease should be deleted from the cluster on uninstall
	HelmProjectOperatorHelmApiVersionLabel = "helm.cattle.io/helm-api-version"
)

func GetHelmResourceLabels(projectID, helmApiVersion string) map[string]string {
	labels := GetCommonLabels(projectID)
	labels[HelmProjectOperatorHelmApiVersionLabel] = strings.SplitN(helmApiVersion, "/", 2)[0]
	return labels
}

// RoleBindings (created for Default K8s ClusterRole RBAC aggregation)

const (
	// HelmProjectOperatorProjectHelmChartRoleBindingLabel is a label that identifies a RoleBinding as one that has been created in response to a ProjectHelmChart role
	// The value of this label will be the release name of the Helm chart, which will be used to identify which ProjectHelmChart's enqueue should resynchronize this.
	HelmProjectOperatorProjectHelmChartRoleBindingLabel = "helm.cattle.io/project-helm-chart-role-binding"
)
