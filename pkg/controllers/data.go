package controllers

import (
	"fmt"
	"strings"

	"github.com/aiyengar2/helm-project-operator/pkg/controllers/common"
	"github.com/aiyengar2/helm-project-operator/pkg/controllers/namespace"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func addChartDataWrapper(helmApiVersion, questionsYaml, valuesYaml string, appCtx *appContext) namespace.OnNamespaceFunc {
	return func(namespace *corev1.Namespace) error {
		return addChartData(namespace, helmApiVersion, questionsYaml, valuesYaml, appCtx)
	}
}

func addChartData(namespace *corev1.Namespace, helmApiVersion, questionsYaml, valuesYaml string, appCtx *appContext) error {
	if namespace == nil {
		return nil
	}
	return appCtx.Apply.
		WithSetID(fmt.Sprintf("%s-data", namespace.Name)).
		WithCacheTypes(appCtx.Core.ConfigMap()).
		ApplyObjects(getConfigMap(namespace.Name, helmApiVersion, valuesYaml, questionsYaml))
}

func getConfigMap(namespace, helmApiVersion, valuesYaml, questionsYaml string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      strings.ReplaceAll(helmApiVersion, "/", "."),
			Namespace: namespace,
			Labels: map[string]string{
				common.HelmProjectOperatedLabel: "true",
			},
		},
		Data: map[string]string{
			"values.yaml":    valuesYaml,
			"questions.yaml": questionsYaml,
		},
	}
}
