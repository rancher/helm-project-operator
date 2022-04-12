package controllers

import (
	"context"
	"fmt"
	"strings"

	"github.com/aiyengar2/helm-project-operator/pkg/controllers/common"
	"github.com/aiyengar2/helm-project-operator/pkg/controllers/namespace"
	"github.com/rancher/wrangler/pkg/relatedresource"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func addChartDataWrapper(ctx context.Context, helmApiVersion, questionsYaml, valuesYaml string, appCtx *appContext) namespace.OnNamespaceFunc {
	// setup watch on configmap to trigger namespace enqueue
	relatedresource.WatchClusterScoped(ctx, "sync-namespace-data",
		func(namespace, name string, obj runtime.Object) ([]relatedresource.Key, error) {
			// enqueue based on configMap name match
			if name != getConfigMapName(helmApiVersion) {
				return nil, nil
			}
			return []relatedresource.Key{{
				Name: namespace,
			}}, nil
		},
		appCtx.Core.Namespace(), appCtx.Core.ConfigMap(),
	)

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
			Name:      getConfigMapName(helmApiVersion),
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

func getConfigMapName(helmApiVersion string) string {
	return strings.ReplaceAll(helmApiVersion, "/", ".")
}
