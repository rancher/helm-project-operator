package hardened

import (
	"github.com/aiyengar2/helm-project-operator/pkg/controllers/common"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Note: each resource created here should have a resolver set in resolvers.go
// The only exception is namespaces since those are handled by the main controller OnChange

var (
	defaultServiceAccountName           = "default"
	defaultAutomountServiceAccountToken = false // ensures that all pods need to have service account attached to get permissions

	defaultNetworkPolicyName = "hpo-generated-default"
	defaultNetworkPolicySpec = networkingv1.NetworkPolicySpec{
		PodSelector: metav1.LabelSelector{},                         // select all pods
		Ingress:     []networkingv1.NetworkPolicyIngressRule{},      // networking policy limits all ingress
		Egress:      []networkingv1.NetworkPolicyEgressRule{},       // network limits all egress
		PolicyTypes: []networkingv1.PolicyType{"Ingress", "Egress"}, // applies to both ingress and egress
	}
)

func (h *handler) getDefaultServiceAccount(namespace *v1.Namespace) *corev1.ServiceAccount {
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultServiceAccountName,
			Namespace: namespace.Name,
			Labels: map[string]string{
				common.HelmProjectOperatedLabel: "true",
			},
		},
		AutomountServiceAccountToken: &defaultAutomountServiceAccountToken,
	}
	if h.opts.ServiceAccount != nil {
		if h.opts.ServiceAccount.Secrets != nil {
			serviceAccount.Secrets = h.opts.ServiceAccount.Secrets
		}
		if h.opts.ServiceAccount.ImagePullSecrets != nil {
			serviceAccount.ImagePullSecrets = h.opts.ServiceAccount.ImagePullSecrets
		}
		if h.opts.ServiceAccount.AutomountServiceAccountToken != nil {
			serviceAccount.AutomountServiceAccountToken = h.opts.ServiceAccount.AutomountServiceAccountToken
		}
	}
	return serviceAccount
}

func (h *handler) getNetworkPolicy(namespace *v1.Namespace) *networkingv1.NetworkPolicy {
	networkPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultNetworkPolicyName,
			Namespace: namespace.Name,
			Labels: map[string]string{
				common.HelmProjectOperatedLabel: "true",
			},
		},
		Spec: defaultNetworkPolicySpec,
	}
	if h.opts.NetworkPolicy != nil {
		networkPolicy.Spec = networkingv1.NetworkPolicySpec(*h.opts.NetworkPolicy)
	}
	return networkPolicy
}
