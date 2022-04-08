package namespace

import (
	"context"
	"fmt"
	"sync"

	"github.com/aiyengar2/helm-project-operator/pkg/controllers/common"
	helmproject "github.com/aiyengar2/helm-project-operator/pkg/generated/controllers/helm.cattle.io/v1alpha1"
	"github.com/rancher/wrangler/pkg/apply"
	core "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	ProjectSystemNamespaceFmt = "cattle-project-%s-system"
)

type handler struct {
	projectLabel            string
	systemProjectLabelValue string

	systemNamespaces map[string]bool
	systemMapLock    sync.RWMutex

	projectRegistrationNamespaces map[string]*v1.Namespace
	projectRegistrationMapLock    sync.RWMutex

	namespaces            core.NamespaceController
	namespaceCache        core.NamespaceCache
	projectHelmCharts     helmproject.ProjectHelmChartController
	projectHelmChartCache helmproject.ProjectHelmChartCache

	apply apply.Apply
}

const (
	ProjectRegistrationNamespaceByNamespace = "helm.cattle.io/project-registration-namespace-by-namespace"
)

func Register(
	ctx context.Context,
	apply apply.Apply,
	projectLabel string,
	systemProjectLabelValue string,
	systemNamespaceList []string,
	namespaces core.NamespaceController,
	namespaceCache core.NamespaceCache,
	projectHelmCharts helmproject.ProjectHelmChartController,
	projectHelmChartCache helmproject.ProjectHelmChartCache,
) ProjectGetter {
	// initialize systemNamespaceList as a map

	systemNamespaces := make(map[string]bool)
	for _, namespace := range systemNamespaceList {
		systemNamespaces[namespace] = true
	}

	// note: we never delete namespaces that are created since it's possible that the user may want to leave them around
	// on remove, we only output a log that says that the user should clean it up and add an annotation that it is orphaned
	apply = apply.
		WithSetID("project-registration-namespace-applier").
		WithCacheTypes(namespaces).
		WithNoDeleteGVK(namespaces.GroupVersionKind())

	h := &handler{
		projectLabel:                  projectLabel,
		systemProjectLabelValue:       systemProjectLabelValue,
		systemNamespaces:              systemNamespaces,
		projectRegistrationNamespaces: make(map[string]*v1.Namespace),
		namespaces:                    namespaces,
		namespaceCache:                namespaceCache,
		projectHelmCharts:             projectHelmCharts,
		projectHelmChartCache:         projectHelmChartCache,
		apply:                         apply,
	}
	namespaces.OnChange(ctx, "on-namespace-change", h.OnChange)
	namespaces.OnRemove(ctx, "on-namespace-change", h.OnChange)

	namespaceCache.AddIndexer(ProjectRegistrationNamespaceByNamespace, h.namespaceToProjectRegistrationNamespace)

	namespaceList, err := namespaces.List(metav1.ListOptions{})
	if err != nil {
		logrus.Panicf("unable to list namespaces to enqueue all Helm charts")
	}
	if namespaceList != nil {
		for _, ns := range namespaceList.Items {
			namespaces.Enqueue(ns.Name)
		}
	}

	return NewLabelBasedProjectGetter(projectLabel, h.isProjectRegistrationNamespace, h.isSystemNamespace, namespaceCache)
}

func (h *handler) namespaceToProjectRegistrationNamespace(namespace *v1.Namespace) ([]string, error) {
	projectID, inProject := h.getProjectIDFromNamespaceLabels(namespace)
	if !inProject {
		// nothing to do
		return nil, nil
	}
	return []string{projectID}, nil
}

func (h *handler) OnChange(name string, namespace *v1.Namespace) (*v1.Namespace, error) {
	if namespace == nil {
		return namespace, nil
	}
	if h.isSystemNamespace(namespace) {
		// nothing to do, we always ignore system namespaces
		return namespace, nil
	}
	if h.isProjectRegistrationNamespace(namespace) {
		// ensure that we are working with the projectRegistrationNamespace that we expect, not the one we found
		namespace, err := h.getProjectRegistrationNamespaceFromNamespace(namespace)
		if err != nil {
			return namespace, err
		}
		if namespace.DeletionTimestamp == nil {
			// projectRegistrationNamespace was changed
			// always apply what we think the namespace should be on seeing changes to the namespace
			return namespace, h.apply.ApplyObjects(namespace)
		}
		// projectRegistrationNamespace was removed, so we should re-enqueue any namespaces tied to it
		projectID, ok := namespace.Labels[h.projectLabel]
		if !ok {
			return namespace, fmt.Errorf("could not find project that projectRegistrationNamespace %s is tied to", namespace.Name)
		}
		projectNamespaces, err := h.namespaceCache.GetByIndex(ProjectRegistrationNamespaceByNamespace, projectID)
		if err != nil {
			return namespace, err
		}
		for _, ns := range projectNamespaces {
			if h.isProjectRegistrationNamespace(ns) {
				continue
			}
			h.namespaces.Enqueue(ns.Name)
		}
		// ensure that we're no longer tracking this namespace in memory if it is gone
		// if it gets recreated, it will be set back into memory anyways
		h.deleteProjectSystemNamespace(namespace)
		return namespace, nil
	}

	// get the project ID and generate the namespace object to be applied
	projectID, inProject := h.getProjectIDFromNamespaceLabels(namespace)
	if !inProject {
		return namespace, nil
	}

	// ensure that the projectRegistrationNamespace created from this projectID is valid
	projectSystemNamespaceName := fmt.Sprintf(ProjectSystemNamespaceFmt, projectID)
	if len(projectSystemNamespaceName) > 63 {
		// ensure that we don't try to create a namespace with too big of a name
		logrus.Errorf("could not apply namespace with name %s: name is above 63 characters", projectSystemNamespaceName)
		return namespace, nil
	}

	// define the expected projectRegistrationNamespace
	projectRegistrationNamespace := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: projectSystemNamespaceName,
			Labels: map[string]string{
				common.HelmProjectOperatedLabel: "true",
				h.projectLabel:                  projectID,
			},
		},
	}

	// ensure that all ProjectHelmCharts are re-enqueued within this projectRegistrationNamespace
	projectHelmCharts, err := h.projectHelmChartCache.List(projectSystemNamespaceName, labels.Everything())
	if err != nil {
		return namespace, fmt.Errorf("unable to re-enqueue ProjectHelmCharts on reconciling change to namespace %s in project %s", namespace, projectID)
	}
	for _, projectHelmChart := range projectHelmCharts {
		h.projectHelmCharts.Enqueue(projectHelmChart.Namespace, projectHelmChart.Name)
	}

	// Calculate whether to add the orphaned label
	projectNamespaces, err := h.namespaceCache.GetByIndex(ProjectRegistrationNamespaceByNamespace, projectID)
	if err != nil {
		return namespace, err
	}
	var numNamespaces int
	for _, ns := range projectNamespaces {
		if h.isProjectRegistrationNamespace(ns) {
			continue
		}
		numNamespaces++
	}
	if numNamespaces == 0 {
		// add orphaned label and trigger a warning
		projectRegistrationNamespace.Labels[common.HelmProjectOperatedOrphanedLabel] = "true"
	}

	// set the projectRegistrationNamespace in memory and trigger the apply
	h.setProjectRegistrationNamespace(projectRegistrationNamespace)
	return namespace, h.apply.ApplyObjects(projectRegistrationNamespace)
}

func (h *handler) isProjectRegistrationNamespace(namespace *v1.Namespace) bool {
	if namespace == nil {
		return false
	}
	h.projectRegistrationMapLock.RLock()
	defer h.projectRegistrationMapLock.RUnlock()
	_, exists := h.projectRegistrationNamespaces[namespace.Name]
	return exists
}

func (h *handler) getProjectRegistrationNamespaceFromNamespace(projectRegistrationNamespace *v1.Namespace) (*v1.Namespace, error) {
	if projectRegistrationNamespace == nil {
		return nil, fmt.Errorf("cannot get projectRegistrationNamespace from nil projectRegistrationNamespace")
	}
	h.projectRegistrationMapLock.RLock()
	defer h.projectRegistrationMapLock.RUnlock()
	ns, exists := h.projectRegistrationNamespaces[projectRegistrationNamespace.Name]
	if !exists {
		return nil, fmt.Errorf("%s is not a projectRegistrationNamespace", projectRegistrationNamespace.Name)
	}
	return ns, nil
}

func (h *handler) isSystemNamespace(systemNamespace *v1.Namespace) bool {
	h.systemMapLock.RLock()
	_, exists := h.systemNamespaces[systemNamespace.Name]
	h.systemMapLock.RUnlock()
	if exists {
		return true
	}
	if len(h.systemProjectLabelValue) != 0 {
		// check if labels indicate this is a system project
		projectID, inProject := h.getProjectIDFromNamespaceLabels(systemNamespace)
		return inProject && projectID == h.systemProjectLabelValue
	}
	return false
}

func (h *handler) setProjectRegistrationNamespace(projectRegistrationNamespace *v1.Namespace) {
	h.projectRegistrationMapLock.Lock()
	defer h.projectRegistrationMapLock.Unlock()
	h.projectRegistrationNamespaces[projectRegistrationNamespace.Name] = projectRegistrationNamespace
}

func (h *handler) deleteProjectSystemNamespace(projectSystemNamespace *v1.Namespace) {
	h.projectRegistrationMapLock.Lock()
	defer h.projectRegistrationMapLock.Unlock()
	delete(h.projectRegistrationNamespaces, projectSystemNamespace.Name)
}

func (h *handler) getProjectIDFromNamespaceLabels(namespace *v1.Namespace) (string, bool) {
	labels := namespace.GetLabels()
	if labels == nil {
		return "", false
	}
	projectID, namespaceInProject := labels[h.projectLabel]
	return projectID, namespaceInProject
}
