package operator

import (
	"github.com/rancher/helm-project-operator/pkg/controllers"
	"github.com/rancher/helm-project-operator/pkg/crd"
	"github.com/rancher/helm-project-operator/pkg/projectoperator"
	"github.com/rancher/wrangler/pkg/ratelimit"
)

// Init sets up a new Helm Project Operator with the provided options and configuration
func Init(p projectoperator.ProjectOperator) error {
	if err := p.Validate(); err != nil {
		return err
	}

	clientConfig, err := p.ClientConfig().ClientConfig()
	if err != nil {
		return err
	}
	clientConfig.RateLimiter = ratelimit.None

	if err := crd.Create(p.Context(), clientConfig); err != nil {
		return err
	}

	return controllers.Register(p)
}
