package project

import (
	"fmt"

	v1alpha1 "github.com/aiyengar2/helm-project-operator/pkg/apis/helm.cattle.io/v1alpha1"
	"github.com/aiyengar2/helm-project-operator/pkg/controllers/common"
)

func (h *handler) getCleanupStatus(projectHelmChart *v1alpha1.ProjectHelmChart, projectHelmChartStatus v1alpha1.ProjectHelmChartStatus) v1alpha1.ProjectHelmChartStatus {
	return v1alpha1.ProjectHelmChartStatus{
		Status: "AwaitingOperatorRedeployment",
		StatusMessage: fmt.Sprintf(
			"ProjectHelmChart was marked with label %s=true, which indicates that the resource should be cleaned up "+
				"until the Project Operator that responds to ProjectHelmCharts in %s with spec.helmApiVersion=%s "+
				"is redeployed onto the cluster. On redeployment, this label will automatically be removed by the operator.",
			common.HelmProjectOperatedCleanupLabel, projectHelmChart.Namespace, projectHelmChart.Spec.HelmApiVersion,
		),
	}
}

func (h *handler) getUnableToCreateHelmReleaseStatus(projectHelmChart *v1alpha1.ProjectHelmChart, projectHelmChartStatus v1alpha1.ProjectHelmChartStatus, err error) v1alpha1.ProjectHelmChartStatus {
	releaseNamespace, releaseName := h.getReleaseNamespaceAndName(projectHelmChart)
	return v1alpha1.ProjectHelmChartStatus{
		Status: "UnableToCreateHelmRelease",
		StatusMessage: fmt.Sprintf(
			"Unable to create a release (%s/%s) for ProjectHelmChart: %s",
			releaseName, releaseNamespace, err,
		),
	}
}

func (h *handler) getNoTargetNamespacesStatus(projectHelmChart *v1alpha1.ProjectHelmChart, projectHelmChartStatus v1alpha1.ProjectHelmChartStatus) v1alpha1.ProjectHelmChartStatus {
	return v1alpha1.ProjectHelmChartStatus{
		Status:        "NoTargetProjectNamespaces",
		StatusMessage: "There are no project namespaces to deploy a ProjectHelmChart.",
	}
}

func (h *handler) getValuesParseErrorStatus(projectHelmChart *v1alpha1.ProjectHelmChart, projectHelmChartStatus v1alpha1.ProjectHelmChartStatus, err error) v1alpha1.ProjectHelmChartStatus {
	// retain existing status if possible
	projectHelmChartStatus.Status = "UnableToParseValues"
	projectHelmChartStatus.StatusMessage = fmt.Sprintf("Unable to convert provided spec.values into valid configuration of ProjectHelmChart: %s", err)
	return projectHelmChartStatus
}

func (h *handler) getWaitingForDashboardValuesStatus(projectHelmChart *v1alpha1.ProjectHelmChart, projectHelmChartStatus v1alpha1.ProjectHelmChartStatus) v1alpha1.ProjectHelmChartStatus {
	// retain existing status
	projectHelmChartStatus.Status = "WaitingForDashboardValues"
	projectHelmChartStatus.StatusMessage = "Waiting for status.dashboardValues content to be provided by the deployed Helm release, but HelmChart and HelmRelease should be deployed."
	projectHelmChartStatus.DashboardValues = nil
	return projectHelmChartStatus
}

func (h *handler) getDeployedStatus(projectHelmChart *v1alpha1.ProjectHelmChart, projectHelmChartStatus v1alpha1.ProjectHelmChartStatus) v1alpha1.ProjectHelmChartStatus {
	// retain existing status
	projectHelmChartStatus.Status = "Deployed"
	projectHelmChartStatus.StatusMessage = "ProjectHelmChart has been successfully deployed!"
	return projectHelmChartStatus
}
