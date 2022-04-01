package operator

import (
	"context"

	"github.com/aiyengar2/helm-project-operator/pkg/controllers"
	"github.com/aiyengar2/helm-project-operator/pkg/controllers/common"
	"github.com/aiyengar2/helm-project-operator/pkg/crd"
	"github.com/rancher/wrangler/pkg/ratelimit"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/tools/clientcmd"
)

func Init(ctx context.Context, systemNamespace string, cfg clientcmd.ClientConfig, opts common.Options) error {
	if systemNamespace == "" {
		logrus.Warnf("No system namespace was provided. Assuming kube-system is where this controller is running...")
		systemNamespace = "kube-system"
	}
	if err := opts.Validate(); err != nil {
		return err
	}

	clientConfig, err := cfg.ClientConfig()
	if err != nil {
		return err
	}
	clientConfig.RateLimiter = ratelimit.None

	if err := crd.Create(ctx, clientConfig); err != nil {
		return err
	}

	return controllers.Register(ctx, systemNamespace, cfg, opts)
}
