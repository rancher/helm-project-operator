package namespace

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aiyengar2/helm-project-operator/pkg/controllers/common"
	helmproject "github.com/aiyengar2/helm-project-operator/pkg/generated/controllers/helm.cattle.io/v1alpha1"
	"github.com/rancher/wrangler/pkg/apply"
	corecontrollers "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type OnNamespaceFunc func(*v1.Namespace) error

type handler struct {
	projectLabel            string
	systemProjectLabelValue string
	clusterID               string

	systemNamespaces map[string]bool
	systemMapLock    sync.RWMutex

	projectRegistrationNamespaces map[string]*v1.Namespace
	projectRegistrationMapLock    sync.RWMutex

	namespaces            corecontrollers.NamespaceController
	namespaceCache        corecontrollers.NamespaceCache
	projectHelmCharts     helmproject.ProjectHelmChartController
	projectHelmChartCache helmproject.ProjectHelmChartCache

	apply apply.Apply

	onProjectRegistrationNamespace OnNamespaceFunc
}

const (
	NamespacesByProjectID = "helm.cattle.io/namespaces-by-project-id"
)

func Register(
	ctx context.Context,
	apply apply.Apply,
	projectLabel string,
	systemProjectLabelValue string,
	clusterID string,
	systemNamespaceList []string,
	namespaces corecontrollers.NamespaceController,
	namespaceCache corecontrollers.NamespaceCache,
	projectHelmCharts helmproject.ProjectHelmChartController,
	projectHelmChartCache helmproject.ProjectHelmChartCache,
	onProjectRegistrationNamespace OnNamespaceFunc,
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
		apply:                          apply,
		projectLabel:                   projectLabel,
		systemProjectLabelValue:        systemProjectLabelValue,
		clusterID:                      clusterID,
		systemNamespaces:               systemNamespaces,
		projectRegistrationNamespaces:  make(map[string]*v1.Namespace),
		namespaces:                     namespaces,
		namespaceCache:                 namespaceCache,
		projectHelmCharts:              projectHelmCharts,
		projectHelmChartCache:          projectHelmChartCache,
		onProjectRegistrationNamespace: onProjectRegistrationNamespace,
	}

	namespaces.OnChange(ctx, "on-namespace-change", h.OnChange)
	namespaces.OnRemove(ctx, "on-namespace-remove", h.OnChange)

	namespaceCache.AddIndexer(NamespacesByProjectID, h.namespaceToProjectID)

	err := h.initProjectRegistrationNamespaces()
	if err != nil {
		logrus.Fatal(err)
	}

	return NewLabelBasedProjectGetter(projectLabel, h.isProjectRegistrationNamespace, h.isSystemNamespace, namespaces)
}

func (h *handler) initProjectRegistrationNamespaces() error {
	namespaceList, err := h.namespaces.List(metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("unable to list namespaces to enqueue all Helm charts: %s", err)
	}
	if namespaceList != nil {
		logrus.Infof("Identifying and registering projectRegistrationNamespaces...")
		// trigger the OnChange events for all namespaces before returning on a register
		//
		// this ensures that registration will create projectRegistrationNamespaces and
		// have isProjectRegistration and isSystemNamespace up to sync before it provides
		// the ProjectGetter interface to other controllers that need it.
		//
		// Q: Why don't we use Enqueue here?
		//
		// Enqueue will add it to the workqueue but there's no guarentee the namespace's processing
		// will happen before this function exits, which is what we need to guarentee here.
		// As a result, we explicitly call OnChange here to force the apply to happen and wait for it to finish
		for _, ns := range namespaceList.Items {
			_, err := h.OnChange(ns.Name, &ns)
			if err != nil {
				// encountered some error, just fail to start
				// Possible TODO: Perhaps we should add a backoff retry here?
				return fmt.Errorf("unable to initialize projectRegistrationNamespaces before starting other handlers that utilize ProjectGetter: %s", err)
			}
		}
	}
	return nil
}

func (h *handler) namespaceToProjectID(namespace *v1.Namespace) ([]string, error) {
	if h.isSystemNamespace(namespace) {
		return nil, nil
	}
	if h.isProjectRegistrationNamespace(namespace) {
		return nil, nil
	}
	if namespace.Labels[common.HelmProjectOperatedLabel] == "true" {
		// always ignore Helm Project Operated namespaces since those are only
		// to be scoped to namespaces that are project registration namespaces
		return nil, nil
	}
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

	switch {
	// note: the check for a project registration namespace must happen before
	// we check for whether it is a system namespace to address the scenario where
	// the 'projectLabel: systemProjectLabelValue' is added to the project registration
	// namespace, which will cause it to be ignored and left in the System Project unless
	// we apply the ProjectRegistrationNamespace logic first.
	case h.isProjectRegistrationNamespace(namespace):
		err := h.enqueueProjectNamespaces(namespace)
		if err != nil {
			return namespace, err
		}
		if namespace.DeletionTimestamp != nil {
			h.deleteProjectRegistrationNamespace(namespace)
		}
		return namespace, nil
	case h.isSystemNamespace(namespace):
		// nothing to do, we always ignore system namespaces
		return namespace, nil
	default:
		err := h.applyProjectRegistrationNamespaceForNamespace(namespace)
		if err != nil {
			return namespace, err
		}
		return namespace, nil
	}
}

func (h *handler) enqueueProjectNamespaces(projectRegistrationNamespace *v1.Namespace) error {
	// ensure that we are working with the projectRegistrationNamespace that we expect, not the one we found
	expectedNamespace, err := h.getProjectRegistrationNamespaceFromNamespace(projectRegistrationNamespace)
	if err != nil {
		// we no longer expect this namespace to exist, so don't enqueue any namespaces
		return nil
	}
	// projectRegistrationNamespace was removed, so we should re-enqueue any namespaces tied to it
	projectID, ok := expectedNamespace.Labels[h.projectLabel]
	if !ok {
		return fmt.Errorf("could not find project that projectRegistrationNamespace %s is tied to", projectRegistrationNamespace.Name)
	}
	projectNamespaces, err := h.namespaceCache.GetByIndex(NamespacesByProjectID, projectID)
	if err != nil {
		return err
	}
	for _, ns := range projectNamespaces {
		h.namespaces.EnqueueAfter(ns.Name, time.Second)
	}
	return nil
}

func (h *handler) updateNamespaceWithHelmOperatorProjectLabel(namespace *v1.Namespace, projectID string, inProject bool) error {
	if namespace.DeletionTimestamp != nil {
		// no need to update a namespace about to be deleted
		return nil
	}
	if len(h.systemProjectLabelValue) == 0 {
		// do nothing, this annotation is irrelevant unless we create release namespaces
		return nil
	}
	if len(projectID) == 0 || !inProject {
		// ensure that the HelmProjectOperatorProjectLabel is removed if added
		if namespace.Labels == nil {
			return nil
		}
		if _, ok := namespace.Labels[common.HelmProjectOperatorProjectLabel]; !ok {
			return nil
		}
		namespaceCopy := namespace.DeepCopy()
		delete(namespaceCopy.Labels, common.HelmProjectOperatorProjectLabel)
		_, err := h.namespaces.Update(namespaceCopy)
		if err != nil {
			return err
		}
	}

	namespaceCopy := namespace.DeepCopy()
	if namespaceCopy.Labels == nil {
		namespaceCopy.Labels = map[string]string{}
	}
	currLabel, ok := namespaceCopy.Labels[common.HelmProjectOperatorProjectLabel]
	if !ok || currLabel != projectID {
		namespaceCopy.Labels[common.HelmProjectOperatorProjectLabel] = projectID
	}
	_, err := h.namespaces.Update(namespaceCopy)
	if err != nil {
		return err
	}
	return nil
}

func (h *handler) applyProjectRegistrationNamespaceForNamespace(namespace *v1.Namespace) error {
	// get the project ID and generate the namespace object to be applied
	projectID, inProject := h.getProjectIDFromNamespaceLabels(namespace)
	err := h.updateNamespaceWithHelmOperatorProjectLabel(namespace, projectID, inProject)
	if err != nil {
		return nil
	}
	if !inProject {
		return nil
	}
	// ensure that the projectRegistrationNamespace created from this projectID is valid
	projectRegistrationNamespaceName := fmt.Sprintf(common.ProjectRegistrationNamespaceFmt, projectID)
	if len(projectRegistrationNamespaceName) > 63 {
		// ensure that we don't try to create a namespace with too big of a name
		logrus.Errorf("could not apply namespace with name %s: name is above 63 characters", projectRegistrationNamespaceName)
		return nil
	}
	if projectRegistrationNamespaceName == namespace.Name {
		// the only way this would happen is if h.isProjectRegistrationNamespace(namespace), which means the
		// the project registration namespace was removed from the cluster after it was orphaned (but still in the project
		// since it has the projectID label on it). In this case, we can safely ignore and continue
		return nil
	}

	projectIDWithClusterID := projectID
	if len(h.clusterID) > 0 {
		projectIDWithClusterID = fmt.Sprintf("%s:%s", h.clusterID, projectID)
	}

	// define the expected projectRegistrationNamespace
	projectRegistrationNamespace := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: projectRegistrationNamespaceName,
			Annotations: map[string]string{
				h.projectLabel: projectIDWithClusterID,
			},
			Labels: map[string]string{
				common.HelmProjectOperatedLabel: "true",
				h.projectLabel:                  projectID,
			},
		},
	}

	// Calculate whether to add the orphaned label
	projectNamespaces, err := h.namespaceCache.GetByIndex(NamespacesByProjectID, projectID)
	if err != nil {
		return err
	}
	var numNamespaces int
	for _, ns := range projectNamespaces {
		if ns.DeletionTimestamp != nil && ns.Name == namespace.Name {
			// ignore the namespace we are deleting, which can still be in the index
			continue
		}
		numNamespaces++
	}
	if numNamespaces == 0 {
		// add orphaned label and trigger a warning
		projectRegistrationNamespace.Labels[common.HelmProjectOperatedOrphanedLabel] = "true"
	}

	// Trigger the apply and set the projectRegistrationNamespace
	err = h.apply.ApplyObjects(projectRegistrationNamespace)
	if err != nil {
		return err
	}
	err = h.onProjectRegistrationNamespace(projectRegistrationNamespace)
	if err != nil {
		return err
	}
	h.setProjectRegistrationNamespace(projectRegistrationNamespace)

	// ensure that all ProjectHelmCharts are re-enqueued within this projectRegistrationNamespace
	projectHelmCharts, err := h.projectHelmChartCache.List(projectRegistrationNamespaceName, labels.Everything())
	if err != nil {
		return fmt.Errorf("unable to re-enqueue ProjectHelmCharts on reconciling change to namespaces in project %s", projectID)
	}
	for _, projectHelmChart := range projectHelmCharts {
		h.projectHelmCharts.Enqueue(projectHelmChart.Namespace, projectHelmChart.Name)
	}

	return nil
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

func (h *handler) deleteProjectRegistrationNamespace(projectRegistrationNamespace *v1.Namespace) {
	h.projectRegistrationMapLock.Lock()
	defer h.projectRegistrationMapLock.Unlock()
	delete(h.projectRegistrationNamespaces, projectRegistrationNamespace.Name)
}

func (h *handler) getProjectIDFromNamespaceLabels(namespace *v1.Namespace) (string, bool) {
	labels := namespace.GetLabels()
	if labels == nil {
		return "", false
	}
	projectID, namespaceInProject := labels[h.projectLabel]
	return projectID, namespaceInProject
}
