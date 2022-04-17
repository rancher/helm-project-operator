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
	// ProjectRegistrationNamespaceFmt is the format used in order to create project registration namespaces if ProjectLabel is provided
	// If SystemProjectLabel is also provided, the project release namespace will be this namespace with `-<ReleaseName>` suffixed, where
	// ReleaseName is provided by the Project Operator that implements Helm Project Operator
	ProjectRegistrationNamespaceFmt = "cattle-project-%s"
)
