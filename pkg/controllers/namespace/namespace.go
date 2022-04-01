package namespace

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/aiyengar2/helm-project-operator/pkg/controllers/common"
	"github.com/rancher/wrangler/pkg/apply"
	core "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	ProjectSystemNamespaceFmt = "cattle-project-%s-system"
)

var (
	NamespaceGVK = schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"}
)

type handler struct {
	projectLabel    string
	systemProjectID string

	systemNamespaces map[string]bool
	systemMapLock    sync.RWMutex

	projectSystemNamespaces map[string]bool
	projectSystemMapLock    sync.RWMutex

	namespaces     core.NamespaceController
	namespaceCache core.NamespaceCache
	namespaceApply apply.Apply
}

func Register(
	ctx context.Context,
	apply apply.Apply,
	projectLabel string,
	systemProjectID string,
	systemNamespaceList []string,
	namespaces core.NamespaceController,
	namespaceCache core.NamespaceCache,
) NamespaceChecker {
	// initialize systemNamespaceList as a map

	systemNamespaces := make(map[string]bool)
	for _, namespace := range systemNamespaceList {
		systemNamespaces[namespace] = true
	}

	namespaceApply := apply.WithCacheTypes(namespaces).WithNoDeleteGVK(NamespaceGVK)

	h := &handler{
		projectLabel:            projectLabel,
		systemProjectID:         systemProjectID,
		systemNamespaces:        systemNamespaces,
		projectSystemNamespaces: make(map[string]bool),
		namespaces:              namespaces,
		namespaceCache:          namespaceCache,
		namespaceApply:          namespaceApply,
	}
	namespaces.OnChange(ctx, "create-namespace", h.OnChange)
	namespaces.OnRemove(ctx, "remove-namespace", h.OnRemove)

	return func(name string) bool {
		return h.isProjectSystemNamespace(&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}})
	}
}

func (h *handler) OnChange(name string, namespace *v1.Namespace) (*v1.Namespace, error) {
	if namespace == nil {
		return namespace, nil
	}
	if namespace.DeletionTimestamp != nil {
		return namespace, nil
	}

	if h.isSystemNamespace(namespace) {
		// nothing to do
		return namespace, nil
	}

	var projectSystemNamespaceName string
	if !h.isProjectSystemNamespace(namespace) {
		// get the true projectSystemNamespace name from the labels of the namespace
		projectID, inProject := h.getProjectIDFromNamespaceLabels(namespace)
		if !inProject {
			// nothing to do
			return namespace, nil
		}
		projectSystemNamespaceName = fmt.Sprintf(ProjectSystemNamespaceFmt, projectID)
	} else {
		projectSystemNamespaceName = name
	}

	if len(projectSystemNamespaceName) > 63 {
		// ensure that we don't try to create a namespace with too big of a name
		logrus.Errorf("could not apply namespace with name %s: name is above 63 characters", projectSystemNamespaceName)
		return namespace, nil
	}

	// create the project system namespace
	namespaceLabels := map[string]string{
		common.HelmProjectOperatedLabel: "true",
	}
	if len(h.systemProjectID) > 0 {
		namespaceLabels[h.projectLabel] = h.systemProjectID
	}
	projectSystemNamespace := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   projectSystemNamespaceName,
			Labels: namespaceLabels,
		},
	}
	h.addProjectSystemNamespace(projectSystemNamespace)
	return namespace, h.namespaceApply.ApplyObjects(projectSystemNamespace)
}

func (h *handler) OnRemove(name string, namespace *v1.Namespace) (*v1.Namespace, error) {
	if h.isSystemNamespace(namespace) {
		// nothing to do; system namespaces will never be tied to projects
		return namespace, nil
	}
	projectID, isProjectNamespace := h.parseProjectID(namespace)
	if !isProjectNamespace {
		// TODO: add cleanup for project system namespace on removal of all project namespaces
		//
		// it's unclear what action to take here as of now since we don't want to automatically clean up these namespaces
		// on seeing no more namespaces tied to the project as it's possible that the user would like to keep those namespaces
		// around while they are temporarily removing namespaces from a project.
		//
		// Therefore, cleaning up namespaces is left as an exercise for the user
		// ensure it gets tracked as something removed
		return namespace, nil
	}

	// get all namespaces tied to this project
	projectLabelSelector, err := labels.Parse(fmt.Sprintf("%s=%s", h.projectLabel, projectID))
	if err != nil {
		return namespace, fmt.Errorf("unable to create project namespace selector: %s", err)
	}
	projectNamespaces, err := h.namespaceCache.List(projectLabelSelector)
	if err != nil {
		return namespace, fmt.Errorf("unable to list project namespaces: %s", err)
	}

	for _, ns := range projectNamespaces {
		// enqueue all namespaces in the project; if none exist, nothing will be enqueued
		// if a project namespace does exist, this will ensure this project system namespace gets recreated
		h.namespaces.Enqueue(ns.Name)
	}
	h.removeProjectSystemNamespace(namespace)

	return namespace, nil
}

func (h *handler) isProjectSystemNamespace(projectSystemNamespace *v1.Namespace) bool {
	h.projectSystemMapLock.RLock()
	defer h.projectSystemMapLock.RUnlock()
	_, exists := h.projectSystemNamespaces[projectSystemNamespace.Name]
	return exists
}

func (h *handler) addProjectSystemNamespace(projectSystemNamespace *v1.Namespace) {
	h.projectSystemMapLock.Lock()
	defer h.projectSystemMapLock.Unlock()
	h.projectSystemNamespaces[projectSystemNamespace.Name] = true
}

func (h *handler) removeProjectSystemNamespace(projectSystemNamespace *v1.Namespace) {
	h.projectSystemMapLock.Lock()
	defer h.projectSystemMapLock.Unlock()
	delete(h.projectSystemNamespaces, projectSystemNamespace.Name)
}

func (h *handler) getProjectIDFromNamespaceLabels(namespace *v1.Namespace) (string, bool) {
	projectID, namespaceInProject := namespace.GetLabels()[h.projectLabel]
	return projectID, namespaceInProject
}

func (h *handler) parseProjectID(projectSystemNamespace *v1.Namespace) (string, bool) {
	if !h.isProjectSystemNamespace(projectSystemNamespace) {
		return "", false
	}
	projectSystemNamespaceFmtSplit := strings.Split(ProjectSystemNamespaceFmt, "%s")
	if len(projectSystemNamespaceFmtSplit) != 2 {
		return "", false
	}
	projectID := projectSystemNamespace.Name
	projectID = strings.TrimPrefix(projectID, projectSystemNamespaceFmtSplit[0])
	projectID = strings.TrimSuffix(projectID, projectSystemNamespaceFmtSplit[1])
	return projectID, true
}

func (h *handler) isSystemNamespace(systemNamespace *v1.Namespace) bool {
	h.systemMapLock.RLock()
	_, exists := h.systemNamespaces[systemNamespace.Name]
	h.systemMapLock.RUnlock()
	if exists {
		return true
	}
	if h.isProjectSystemNamespace(systemNamespace) {
		// project system namespaces will have the systemProjectID, but are not system namespaces
		return false
	}
	if len(h.systemProjectID) != 0 {
		// check if labels indicate this is a system project
		projectID, inProject := h.getProjectIDFromNamespaceLabels(systemNamespace)
		return inProject && projectID == h.systemProjectID
	}
	return false
}
