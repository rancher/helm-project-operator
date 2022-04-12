package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ProjectHelmChart struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ProjectHelmChartSpec   `json:"spec"`
	Status            ProjectHelmChartStatus `json:"status"`
}

type ProjectHelmChartSpec struct {
	HelmApiVersion           string                `json:"helmApiVersion"`
	ProjectNamespaceSelector *metav1.LabelSelector `json:"projectNamespaceSelector"`
	Values                   GenericMap            `json:"values"`
}

type ProjectHelmChartStatus struct {
	RancherValues   GenericMap `json:"rancherValues"`
	DashboardValues GenericMap `json:"dashboardValues"`

	ProjectHelmChartStatus        string `json:"projectHelmChartStatus"`
	ProjectHelmChartStatusMessage string `json:"projectHelmChartStatusMessage"`

	ProjectSystemNamespace  string   `json:"projectSystemNamespace"`
	ProjectReleaseNamespace string   `json:"projectReleaseNamespace"`
	ProjectNamespaces       []string `json:"projectNamespaces"`
}
