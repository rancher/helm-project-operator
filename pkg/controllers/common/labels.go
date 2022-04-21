package common

const (
	// HelmProjectOperatedLabel marks all HelmCharts, HelmReleases, and namespaces created by this operator
	HelmProjectOperatedLabel = "helm.cattle.io/helm-project-operated"
	// HelmProjectOperatedOrphanedLabel marks all auto-generated namespaces that no longer have resources tracked
	// by this operator; if a namespace has this label, it is safe to delete
	HelmProjectOperatedOrphanedLabel = "helm.cattle.io/helm-project-operator-orphaned"
	// HelmProjectOperatedCleanupLabel is a label attached to ProjectHelmCharts to facilitate cleanup; all ProjectHelmCharts
	// with this label will have their HelmCharts and HelmReleases cleaned up until the next time the Operator is deployed;
	// on redeploying the operator, this label will automatically be removed from all ProjectHelmCharts deployed in the cluster.
	HelmProjectOperatedCleanupLabel = "helm.cattle.io/helm-project-operator-cleanup"
	// HelmProjectOperatorProjectLabel is applied to all namespaces targeted by a project only if SystemProjectLabelValue and
	// ProjectLabel are provided, in which case the release namespace of the HelmChart that is deployed will be auto-generated
	// and imported into the system project; since the value of the provided ProjectLabel will not match the value of the ProjectLabel
	// on the generated namespace, this label needs to be added to create a consistent set of labels for global.cattle.projectNamespaceSelector
	// to be able to target
	HelmProjectOperatorProjectLabel = "helm.cattle.io/projectId"
	// HelmProjectOperatorDashboardValuesConfigMapLabel is a label that identifies a ConfigMap that should be merged into status.dashboardValues when available
	// The value of this label will be the release name of the Helm chart, which will be used to identify which ProjectHelmChart's status needs to be updated.
	HelmProjectOperatorDashboardValuesConfigMapLabel = "helm.cattle.io/dashboard-values-configmap"
	// HelmProjectOperatorProjectHelmChartRoleBindingLabel is a label that identifies a RoleBinding as one that has been created in response to a ProjectHelmChart role
	// The value of this label will be the release name of the Helm chart, which will be used to identify which ProjectHelmChart's enqueue should resynchronize this.
	HelmProjectOperatorProjectHelmChartRoleBindingLabel = "helm.cattle.io/project-helm-chart-role-binding"
	// HelmProjectOperatorProjectHelmChartRoleLabel is a label that identifies a Role as one that needs RoleBindings to be managed by the Helm Project Operator
	// The value of this label will be the release name of the Helm chart, which will be used to identify which ProjectHelmChart's enqueue should resynchronize this.
	HelmProjectOperatorProjectHelmChartRoleLabel = "helm.cattle.io/project-helm-chart-role"
	// HelmProjectOperatorProjectHelmChartRoleAggregateFromLabel is a label that identifies which subjects should be bound to the Project Helm Chart Role
	// The value of this label will be the name of the default k8s ClusterRoles (cluster-admin, admin, edit, view). For the provided ClusterRole,
	// the operator will automatically create a RoleBinding in the Project Release Namespace binding all subjects who have that permission across all namespaces in the project
	// to the Role that contains this label. This label will only be viewed if the Role has HelmProjectOperatorProjectHelmChartRoleLabel set as well
	HelmProjectOperatorProjectHelmChartRoleAggregateFromLabel = "helm.cattle.io/project-helm-chart-role-aggregate-from"
	// HelmProjectOperatorHelmApiVersionLabel is a label that identifies the HelmApiVersion that a HelmChart or HelmRelease is tied to
	// This is used to identify whether a HelmChart or HelmRelease should be deleted from the cluster on uninstall
	HelmProjectOperatorHelmApiVersionLabel = "helm.cattle.io/helm-api-version"
)
