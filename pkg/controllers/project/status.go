package project

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aiyengar2/helm-project-operator/pkg/apis/helm.cattle.io/v1alpha1"
	"github.com/aiyengar2/helm-project-operator/pkg/controllers/common"
	"github.com/rancher/wrangler/pkg/data"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func (h *handler) getDeployedStatus(projectHelmChart *v1alpha1.ProjectHelmChart, projectHelmChartStatus v1alpha1.ProjectHelmChartStatus) v1alpha1.ProjectHelmChartStatus {
	dashboardValues, err := h.getDashboardValuesFromConfigmaps(projectHelmChart)
	if err != nil {
		// retain existing status
		projectHelmChartStatus.Status = "UnableToParseStatus"
		projectHelmChartStatus.StatusMessage = "Unable to parse status for status.dashboardValues, but HelmChart and HelmRelease should be deployed."
		return projectHelmChartStatus
	}
	if len(dashboardValues) == 0 {
		// retain existing status
		projectHelmChartStatus.Status = "WaitingForDashboardValues"
		projectHelmChartStatus.StatusMessage = "Waiting for status.dashboardValues content to be provided by the deployed Helm release, but HelmChart and HelmRelease should be deployed."
		return projectHelmChartStatus
	}
	// retain existing status
	projectHelmChartStatus.DashboardValues = dashboardValues
	projectHelmChartStatus.Status = "Deployed"
	projectHelmChartStatus.StatusMessage = "ProjectHelmChart has been successfully deployed!"
	return projectHelmChartStatus
}

func (h *handler) getDashboardValuesFromConfigmaps(projectHelmChart *v1alpha1.ProjectHelmChart) (v1alpha1.GenericMap, error) {
	releaseNamespace, releaseName := h.getReleaseNamespaceAndName(projectHelmChart)
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
