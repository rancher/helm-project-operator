package rolebinding

import (
	"sync"

	rbac "k8s.io/api/rbac/v1"
)

type SubjectRoleGetter interface {
	GetSubjects(targetNamespaces []string, k8sRole string) []rbac.Subject
}

type SubjectRoleTracker interface {
	SubjectRoleGetter

	Set(subject rbac.Subject, namespace string, k8sRole string, hasRole bool)
}

func NewSubjectRoleTracker() SubjectRoleTracker {
	return &projectSubjectRoleTracker{
		subjectToNamespaceToRole: make(map[rbac.Subject]map[string]subjectRole),
	}
}

type projectSubjectRoleTracker struct {
	subjectToNamespaceToRole     map[rbac.Subject]map[string]subjectRole
	subjectToNamespaceToRoleLock sync.RWMutex
}

func (t *projectSubjectRoleTracker) Set(subject rbac.Subject, namespace string, k8sRole string, hasRole bool) {
	t.subjectToNamespaceToRoleLock.Lock()
	defer t.subjectToNamespaceToRoleLock.Unlock()
	namespaceToRole, ok := t.subjectToNamespaceToRole[subject]
	if !ok {
		namespaceToRole = make(map[string]subjectRole)

	}
	role, ok := namespaceToRole[namespace]
	if !ok {
		role = subjectRole{}
	}
	role = role.Set(k8sRole, hasRole)

	if role.HasNoRole() {
		// subject has no permissions in this namespace anymore, no need to track in cache
		delete(namespaceToRole, namespace)
	} else {
		namespaceToRole[namespace] = role
	}
	if len(namespaceToRole) == 0 {
		// subject no longer has any permissions
		delete(t.subjectToNamespaceToRole, subject)
	} else {
		t.subjectToNamespaceToRole[subject] = namespaceToRole
	}
}

func (t *projectSubjectRoleTracker) GetSubjects(targetNamespaces []string, k8sRole string) []rbac.Subject {
	t.subjectToNamespaceToRoleLock.RLock()
	defer t.subjectToNamespaceToRoleLock.RUnlock()
	var subjects []rbac.Subject
	for subject, namespaceToRole := range t.subjectToNamespaceToRole {
		shouldAddSubject := true
		for _, expectedNamespace := range targetNamespaces {
			role, ok := namespaceToRole[expectedNamespace]
			if !ok {
				// subject does not have permissions in one of the namespaces
				shouldAddSubject = false
				break
			}
			if !role.Has(k8sRole) {
				// subject does not have this specific permission in one of the namespaces
				shouldAddSubject = false
				break
			}
		}
		// check if user has permissions in all namespaces, e.g. namespace is ""
		clusterRole, ok := namespaceToRole[""]
		if ok {
			shouldAddSubject = shouldAddSubject || clusterRole.Has(k8sRole)
		}
		if !shouldAddSubject {
			// subject is missing this role in some expected namespace
			continue
		}
		// subject has all the necessary permissions so add them to the list
		subjects = append(subjects, subject)
	}
	return subjects
}
