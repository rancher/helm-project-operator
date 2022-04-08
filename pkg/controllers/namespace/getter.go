package namespace

import (
	"fmt"
	"sort"

	"github.com/aiyengar2/helm-project-operator/pkg/apis/helm.cattle.io/v1alpha1"
	corev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type ProjectGetter interface {
	// IsProjectRegistrationNamespace returns whether to watch for ProjectHelmCharts in the provided namespace
	IsProjectRegistrationNamespace(namespace string) (bool, error)

	// IsSystemNamespace returns whether the provided namespace is considered a system namespace
	IsSystemNamespace(namespace string) (bool, error)

	// GetTargetProjectNamespaces returns the list of namespaces that should be targeted for a given ProjectHelmChart
	// Any namespace returned by this should not be a project registration namespace or a system namespace
	GetTargetProjectNamespaces(projectHelmChart *v1alpha1.ProjectHelmChart) ([]string, error)
}

type NamespaceChecker func(namespace *v1.Namespace) bool

// NewLabelBasedProjectGetter returns a ProjectGetter that gets target project namespaces that meet the following criteria:
// 1) Must have the same projectLabel value as the namespace where the ProjectHelmChart lives in
// 2) Must not be a project registration namespace
// 3) Must not be a system namespace
func NewLabelBasedProjectGetter(
	projectLabel string,
	isProjectRegistrationNamespace NamespaceChecker,
	isSystemNamespace NamespaceChecker,
	namespaceCache corev1.NamespaceCache,
) ProjectGetter {
	return &projectGetter{
		namespaceCache: namespaceCache,

		isProjectRegistrationNamespace: isProjectRegistrationNamespace,
		isSystemNamespace:              isSystemNamespace,

		getProjectNamespaces: func(projectHelmChart *v1alpha1.ProjectHelmChart) ([]*v1.Namespace, error) {
			// source of truth is the projectLabel pair that exists on the namespace that the ProjectHelmChart lives within
			namespace, err := namespaceCache.Get(projectHelmChart.Namespace)
			if err != nil {
				return nil, err
			}
			projectLabelValue, ok := namespace.Labels[projectLabel]
			if !ok {
				return nil, fmt.Errorf("could not find value of label %s in namespace %s", projectLabel, namespace.Name)
			}
			return namespaceCache.List(labels.SelectorFromSet(labels.Set{
				projectLabel: projectLabelValue,
			}))
		},
	}
}

// NewSingleNamespaceProjectGetter returns a ProjectGetter that gets target project namespaces that meet the following criteria:
// 1) Must match the labels provided on spec.projectNamespaceSelector of the projectHelmChart in question
// 2) Must not be the registration namespace
// 3) Must not be part of the provided systemNamespaces
func NewSingleNamespaceProjectGetter(registrationNamespace string, systemNamespaces []string, namespaceCache corev1.NamespaceCache) ProjectGetter {
	isSystemNamespace := make(map[string]bool)
	for _, ns := range systemNamespaces {
		isSystemNamespace[ns] = true
	}
	return &projectGetter{
		namespaceCache: namespaceCache,

		isProjectRegistrationNamespace: func(namespace *v1.Namespace) bool {
			// only one registrationNamespace exists
			return namespace.Name == registrationNamespace
		},
		isSystemNamespace: func(namespace *v1.Namespace) bool {
			// only track explicit systemNamespaces
			return isSystemNamespace[namespace.Name]
		},

		getProjectNamespaces: func(projectHelmChart *v1alpha1.ProjectHelmChart) ([]*v1.Namespace, error) {
			// source of truth is the ProjectHelmChart spec.projectNamespaceSelector
			selector, err := metav1.LabelSelectorAsSelector(projectHelmChart.Spec.ProjectNamespaceSelector)
			if err != nil {
				return nil, err
			}
			return namespaceCache.List(selector)
		},
	}
}

type projectGetter struct {
	namespaceCache corev1.NamespaceCache

	isProjectRegistrationNamespace NamespaceChecker
	isSystemNamespace              NamespaceChecker

	getProjectNamespaces func(projectHelmChart *v1alpha1.ProjectHelmChart) ([]*v1.Namespace, error)
}

func (g *projectGetter) IsProjectRegistrationNamespace(namespace string) (bool, error) {
	namespaceObj, err := g.namespaceCache.Get(namespace)
	if err != nil {
		return false, err
	}
	return g.isProjectRegistrationNamespace(namespaceObj), nil
}

func (g *projectGetter) IsSystemNamespace(namespace string) (bool, error) {
	namespaceObj, err := g.namespaceCache.Get(namespace)
	if err != nil {
		return false, err
	}
	return g.isSystemNamespace(namespaceObj), nil
}

func (g *projectGetter) GetTargetProjectNamespaces(projectHelmChart *v1alpha1.ProjectHelmChart) ([]string, error) {
	projectNamespaces, err := g.getProjectNamespaces(projectHelmChart)
	if err != nil {
		return nil, err
	}
	var namespaces []string
	for _, ns := range projectNamespaces {
		if ns == nil {
			continue
		}
		if g.isProjectRegistrationNamespace(ns) || g.isSystemNamespace(ns) {
			continue
		}
		namespaces = append(namespaces, ns.Name)
	}
	sort.Strings(namespaces)
	return namespaces, nil
}
