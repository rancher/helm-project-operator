package hardened

import (
	"context"

	"github.com/aiyengar2/helm-project-operator/pkg/controllers/common"
	"github.com/rancher/wrangler/pkg/apply"
	corecontrollers "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	networkingcontrollers "github.com/rancher/wrangler/pkg/generated/controllers/networking.k8s.io/v1"
	v1 "k8s.io/api/core/v1"
)

type handler struct {
	apply apply.Apply

	opts common.HardeningOptions

	namespaces      corecontrollers.NamespaceController
	namespaceCache  corecontrollers.NamespaceCache
	serviceaccounts corecontrollers.ServiceAccountController
	networkpolicies networkingcontrollers.NetworkPolicyController
}

func Register(
	ctx context.Context,
	apply apply.Apply,
	opts common.HardeningOptions,
	namespaces corecontrollers.NamespaceController,
	namespaceCache corecontrollers.NamespaceCache,
	serviceaccounts corecontrollers.ServiceAccountController,
	networkpolicies networkingcontrollers.NetworkPolicyController,
) {

	apply = apply.
		WithSetID("hardened-hpo-operated-namespace").
		WithCacheTypes(serviceaccounts, networkpolicies)

	h := &handler{
		apply:           apply,
		namespaces:      namespaces,
		namespaceCache:  namespaceCache,
		serviceaccounts: serviceaccounts,
		networkpolicies: networkpolicies,
	}

	h.initResolvers(ctx)

	namespaces.OnChange(ctx, "harden-hpo-operated-namespace", h.OnChange)
}

// Single Namespace Handler

func (h *handler) OnChange(name string, namespace *v1.Namespace) (*v1.Namespace, error) {
	if !common.HasHelmProjectOperatedLabel(namespace.Labels) {
		// only harden operated namespaces
		return namespace, nil
	}
	return namespace, h.apply.WithOwner(namespace).ApplyObjects(
		h.getDefaultServiceAccount(namespace),
		h.getNetworkPolicy(namespace),
	)
}
