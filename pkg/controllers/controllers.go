package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/k3s-io/helm-controller/pkg/controllers/chart"
	k3shelm "github.com/k3s-io/helm-controller/pkg/generated/controllers/helm.cattle.io"
	k3shelmcontroller "github.com/k3s-io/helm-controller/pkg/generated/controllers/helm.cattle.io/v1"
	"github.com/rancher/helm-locker/pkg/controllers/release"
	helmlocker "github.com/rancher/helm-locker/pkg/generated/controllers/helm.cattle.io"
	helmlockercontroller "github.com/rancher/helm-locker/pkg/generated/controllers/helm.cattle.io/v1alpha1"
	"github.com/rancher/helm-locker/pkg/objectset"
	"github.com/rancher/helm-project-operator/pkg/controllers/common"
	"github.com/rancher/helm-project-operator/pkg/controllers/hardened"
	"github.com/rancher/helm-project-operator/pkg/controllers/namespace"
	"github.com/rancher/helm-project-operator/pkg/controllers/project"
	helmproject "github.com/rancher/helm-project-operator/pkg/generated/controllers/helm.cattle.io"
	helmprojectcontroller "github.com/rancher/helm-project-operator/pkg/generated/controllers/helm.cattle.io/v1alpha1"
	"github.com/rancher/helm-project-operator/pkg/projectoperator"
	"github.com/rancher/lasso/pkg/cache"
	"github.com/rancher/lasso/pkg/client"
	"github.com/rancher/lasso/pkg/controller"
	"github.com/rancher/wrangler/pkg/apply"
	batch "github.com/rancher/wrangler/pkg/generated/controllers/batch"
	batchcontroller "github.com/rancher/wrangler/pkg/generated/controllers/batch/v1"
	"github.com/rancher/wrangler/pkg/generated/controllers/core"
	corecontroller "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/generated/controllers/networking.k8s.io"
	networkingcontroller "github.com/rancher/wrangler/pkg/generated/controllers/networking.k8s.io/v1"
	rbac "github.com/rancher/wrangler/pkg/generated/controllers/rbac"
	rbaccontroller "github.com/rancher/wrangler/pkg/generated/controllers/rbac/v1"
	"github.com/rancher/wrangler/pkg/generic"
	"github.com/rancher/wrangler/pkg/leader"
	"github.com/rancher/wrangler/pkg/ratelimit"
	"github.com/rancher/wrangler/pkg/schemes"
	"github.com/rancher/wrangler/pkg/start"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	typedv1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
)

type appContext struct {
	helmprojectcontroller.Interface

	Dynamic    dynamic.Interface
	K8s        kubernetes.Interface
	Core       corecontroller.Interface
	Networking networkingcontroller.Interface

	HelmLocker        helmlockercontroller.Interface
	ObjectSetRegister objectset.LockableRegister
	ObjectSetHandler  *controller.SharedHandler

	HelmController k3shelmcontroller.Interface
	Batch          batchcontroller.Interface
	RBAC           rbaccontroller.Interface

	Apply            apply.Apply
	EventBroadcaster record.EventBroadcaster

	ClientConfig clientcmd.ClientConfig
	starters     []start.Starter
}

func (a *appContext) start(ctx context.Context) error {
	return start.All(ctx, 50, a.starters...)
}

// Register registers all controllers for the Helm Project Operator based on the provided options
// Assumes projectoperator.ProjectOperator is validated
func Register(p projectoperator.ProjectOperator) error {

	// parse values.yaml and questions.yaml from file

	appCtx, err := newContext(p)
	if err != nil {
		return err
	}

	appCtx.EventBroadcaster.StartLogging(logrus.Debugf)
	appCtx.EventBroadcaster.StartRecordingToSink(&typedv1.EventSinkImpl{
		Interface: appCtx.K8s.CoreV1().Events(p.Namespace()),
	})
	recorder := appCtx.EventBroadcaster.NewRecorder(schemes.All, corev1.EventSource{
		Component: "helm-project-operator",
		Host:      p.Options().NodeName,
	})

	if !p.Options().DisableHardening {
		hardeningOpts, err := common.LoadHardeningOptionsFromFile(p.Options().HardeningOptionsFile)
		if err != nil {
			return err
		}
		hardened.Register(
			p.Context(),
			appCtx.Apply,
			hardeningOpts,
			// watches
			appCtx.Core.Namespace(),
			appCtx.Core.Namespace().Cache(),
			// generates
			appCtx.Core.ServiceAccount(),
			appCtx.Networking.NetworkPolicy(),
		)
	}

	projectGetter := namespace.Register(
		p.Context(),
		appCtx.Apply,
		p.Namespace(),
		p.ValuesYaml,
		p.QuestionsYaml,
		p.Options(),
		// watches and generates
		appCtx.Core.Namespace(),
		appCtx.Core.Namespace().Cache(),
		appCtx.Core.ConfigMap(),
		// enqueues
		appCtx.ProjectHelmChart(),
		appCtx.ProjectHelmChart().Cache(),
		appCtx.Dynamic,
	)

	valuesOverride, err := common.LoadValuesOverrideFromFile(p.Options().ValuesOverrideFile)
	if err != nil {
		return err
	}
	project.Register(
		p.Context(),
		p.Namespace(),
		p.Options(),
		valuesOverride,
		appCtx.Apply,
		// watches
		appCtx.ProjectHelmChart(),
		appCtx.ProjectHelmChart().Cache(),
		appCtx.Core.ConfigMap(),
		appCtx.Core.ConfigMap().Cache(),
		appCtx.RBAC.Role(),
		appCtx.RBAC.Role().Cache(),
		appCtx.RBAC.ClusterRoleBinding(),
		appCtx.RBAC.ClusterRoleBinding().Cache(),
		// watches and generates
		appCtx.HelmController.HelmChart(),
		appCtx.HelmLocker.HelmRelease(),
		appCtx.Core.Namespace(),
		appCtx.Core.Namespace().Cache(),
		appCtx.RBAC.RoleBinding(),
		appCtx.RBAC.RoleBinding().Cache(),
		projectGetter,
	)

	if !p.Options().DisableEmbeddedHelmLocker {
		logrus.Infof("Registering embedded Helm Locker...")
		release.Register(
			p.Context(),
			p.Namespace(),
			p.Options().ControllerName,
			appCtx.HelmLocker.HelmRelease(),
			appCtx.HelmLocker.HelmRelease().Cache(),
			appCtx.Core.Secret(),
			appCtx.Core.Secret().Cache(),
			appCtx.K8s,
			appCtx.ObjectSetRegister,
			appCtx.ObjectSetHandler,
			recorder,
		)
	}

	if !p.Options().DisableEmbeddedHelmController {
		logrus.Infof("Registering embedded Helm Controller...")
		chart.Register(
			p.Context(),
			p.Namespace(),
			p.Options().ControllerName,
			appCtx.K8s,
			appCtx.Apply,
			recorder,
			appCtx.HelmController.HelmChart(),
			appCtx.HelmController.HelmChart().Cache(),
			appCtx.HelmController.HelmChartConfig(),
			appCtx.HelmController.HelmChartConfig().Cache(),
			appCtx.Batch.Job(),
			appCtx.Batch.Job().Cache(),
			appCtx.RBAC.ClusterRoleBinding(),
			appCtx.Core.ServiceAccount(),
			appCtx.Core.ConfigMap())
	}

	leader.RunOrDie(
		p.Context(),
		p.Namespace(),
		fmt.Sprintf("helm-project-operator-%s-lock", p.Options().ReleaseName),
		appCtx.K8s,
		func(ctx context.Context) {
			if err := appCtx.start(ctx); err != nil {
				logrus.Fatal(err)
			}
			logrus.Info("All controllers have been started")
		})

	return nil
}

func controllerFactory(rest *rest.Config) (controller.SharedControllerFactory, error) {
	rateLimit := workqueue.NewItemExponentialFailureRateLimiter(5*time.Millisecond, 60*time.Second)
	clientFactory, err := client.NewSharedClientFactory(rest, nil)
	if err != nil {
		return nil, err
	}

	cacheFactory := cache.NewSharedCachedFactory(clientFactory, nil)
	return controller.NewSharedControllerFactory(cacheFactory, &controller.SharedControllerFactoryOptions{
		DefaultRateLimiter: rateLimit,
		DefaultWorkers:     50,
	}), nil
}

func newContext(p projectoperator.ProjectOperator) (*appContext, error) {
	client, err := p.ClientConfig().ClientConfig()
	if err != nil {
		return nil, err
	}
	client.RateLimiter = ratelimit.None

	dynamic, err := dynamic.NewForConfig(client)
	if err != nil {
		return nil, err
	}

	k8s, err := kubernetes.NewForConfig(client)
	if err != nil {
		return nil, err
	}

	discovery, err := discovery.NewDiscoveryClientForConfig(client)
	if err != nil {
		return nil, err
	}

	apply := apply.New(discovery, apply.NewClientFactory(client))

	scf, err := controllerFactory(client)
	if err != nil {
		return nil, err
	}

	// Shared Controllers

	core, err := core.NewFactoryFromConfigWithOptions(client, &generic.FactoryOptions{
		SharedControllerFactory: scf,
	})
	if err != nil {
		return nil, err
	}
	corev := core.Core().V1()

	networking, err := networking.NewFactoryFromConfigWithOptions(client, &generic.FactoryOptions{
		SharedControllerFactory: scf,
	})
	if err != nil {
		return nil, err
	}
	networkingv := networking.Networking().V1()

	// Helm Project Controller

	var namespace string // by default, this is unset so we watch everything
	if len(p.Options().ProjectLabel) == 0 {
		// we only need to watch the systemNamespace
		namespace = p.Namespace()
	}

	helmproject, err := helmproject.NewFactoryFromConfigWithOptions(client, &generic.FactoryOptions{
		SharedControllerFactory: scf,
		Namespace:               namespace,
	})
	if err != nil {
		return nil, err
	}
	helmprojectv := helmproject.Helm().V1alpha1()

	// Helm Locker Controllers - should be scoped to the system namespace only

	objectSet, objectSetRegister, objectSetHandler := objectset.NewLockableRegister("object-set-register", apply, scf, discovery, nil)

	helmlocker, err := helmlocker.NewFactoryFromConfigWithOptions(client, &generic.FactoryOptions{
		SharedControllerFactory: scf,
		Namespace:               p.Namespace(),
	})
	if err != nil {
		return nil, err
	}
	helmlockerv := helmlocker.Helm().V1alpha1()

	// Helm Controllers - should be scoped to the system namespace only

	helm, err := k3shelm.NewFactoryFromConfigWithOptions(client, &generic.FactoryOptions{
		SharedControllerFactory: scf,
		Namespace:               p.Namespace(),
	})
	if err != nil {
		return nil, err
	}
	helmv := helm.Helm().V1()

	batch, err := batch.NewFactoryFromConfigWithOptions(client, &generic.FactoryOptions{
		SharedControllerFactory: scf,
		Namespace:               p.Namespace(),
	})
	if err != nil {
		return nil, err
	}
	batchv := batch.Batch().V1()

	rbac, err := rbac.NewFactoryFromConfigWithOptions(client, &generic.FactoryOptions{
		SharedControllerFactory: scf,
		Namespace:               p.Namespace(),
	})
	if err != nil {
		return nil, err
	}
	rbacv := rbac.Rbac().V1()

	return &appContext{
		Interface: helmprojectv,

		Dynamic:    dynamic,
		K8s:        k8s,
		Core:       corev,
		Networking: networkingv,

		HelmLocker:        helmlockerv,
		ObjectSetRegister: objectSetRegister,
		ObjectSetHandler:  objectSetHandler,

		HelmController: helmv,
		Batch:          batchv,
		RBAC:           rbacv,

		Apply:            apply.WithSetOwnerReference(false, false),
		EventBroadcaster: record.NewBroadcaster(),

		ClientConfig: p.ClientConfig(),
		starters: []start.Starter{
			core,
			networking,
			batch,
			rbac,
			helm,
			objectSet,
			helmlocker,
			helmproject,
		},
	}, nil
}
