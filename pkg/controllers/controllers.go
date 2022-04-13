package controllers

import (
	"context"
	"errors"
	"time"

	"github.com/aiyengar2/helm-locker/pkg/controllers/release"
	helmlocker "github.com/aiyengar2/helm-locker/pkg/generated/controllers/helm.cattle.io"
	helmlockercontrollers "github.com/aiyengar2/helm-locker/pkg/generated/controllers/helm.cattle.io/v1alpha1"
	"github.com/aiyengar2/helm-locker/pkg/objectset"
	"github.com/aiyengar2/helm-project-operator/pkg/controllers/common"
	"github.com/aiyengar2/helm-project-operator/pkg/controllers/namespace"
	"github.com/aiyengar2/helm-project-operator/pkg/controllers/project"
	helmproject "github.com/aiyengar2/helm-project-operator/pkg/generated/controllers/helm.cattle.io"
	"github.com/aiyengar2/helm-project-operator/pkg/generated/controllers/helm.cattle.io/v1alpha1"
	"github.com/k3s-io/helm-controller/pkg/controllers/chart"
	helm "github.com/k3s-io/helm-controller/pkg/generated/controllers/helm.cattle.io"
	helmcontrollers "github.com/k3s-io/helm-controller/pkg/generated/controllers/helm.cattle.io/v1"
	"github.com/rancher/lasso/pkg/cache"
	"github.com/rancher/lasso/pkg/client"
	"github.com/rancher/lasso/pkg/controller"
	"github.com/rancher/wrangler/pkg/apply"
	batch "github.com/rancher/wrangler/pkg/generated/controllers/batch"
	batchcontrollers "github.com/rancher/wrangler/pkg/generated/controllers/batch/v1"
	"github.com/rancher/wrangler/pkg/generated/controllers/core"
	corecontrollers "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	rbac "github.com/rancher/wrangler/pkg/generated/controllers/rbac"
	rbaccontrollers "github.com/rancher/wrangler/pkg/generated/controllers/rbac/v1"
	"github.com/rancher/wrangler/pkg/generic"
	"github.com/rancher/wrangler/pkg/leader"
	"github.com/rancher/wrangler/pkg/ratelimit"
	"github.com/rancher/wrangler/pkg/schemes"
	"github.com/rancher/wrangler/pkg/start"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	typedv1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
)

type appContext struct {
	v1alpha1.Interface

	K8s  kubernetes.Interface
	Core corecontrollers.Interface

	HelmLocker        helmlockercontrollers.Interface
	ObjectSetRegister objectset.LockableObjectSetRegister
	ObjectSetHandler  *controller.SharedHandler

	HelmController helmcontrollers.Interface
	Batch          batchcontrollers.Interface
	RBAC           rbaccontrollers.Interface

	Apply            apply.Apply
	EventBroadcaster record.EventBroadcaster

	ClientConfig clientcmd.ClientConfig
	starters     []start.Starter
}

func (a *appContext) start(ctx context.Context) error {
	return start.All(ctx, 50, a.starters...)
}

func Register(ctx context.Context, systemNamespace string, cfg clientcmd.ClientConfig, opts common.Options) error {
	if len(systemNamespace) == 0 {
		return errors.New("cannot start controllers on system namespace: system namespace not provided")
	}
	// always add the systemNamespace to the systemNamespaces provided
	opts.SystemNamespaces = append(opts.SystemNamespaces, systemNamespace)

	// parse values.yaml and questions.yaml from file
	valuesYaml, questionsYaml, err := parseValuesAndQuestions(opts.ChartContent)
	if err != nil {
		logrus.Fatal(err)
	}

	appCtx, err := newContext(cfg, systemNamespace, opts)
	if err != nil {
		return err
	}

	appCtx.EventBroadcaster.StartLogging(logrus.Debugf)
	appCtx.EventBroadcaster.StartRecordingToSink(&typedv1.EventSinkImpl{
		Interface: appCtx.K8s.CoreV1().Events(systemNamespace),
	})
	recorder := appCtx.EventBroadcaster.NewRecorder(schemes.All, corev1.EventSource{
		Component: "helm-project-operator",
		Host:      opts.NodeName,
	})

	var projectGetter namespace.ProjectGetter
	if len(opts.ProjectLabel) > 0 {
		// add controllers that create dedicated project namespaces
		projectGetter = namespace.Register(ctx,
			appCtx.Apply,
			opts.ProjectLabel,
			opts.SystemProjectLabelValue,
			opts.ClusterID,
			opts.SystemNamespaces,
			appCtx.Core.Namespace(),
			appCtx.Core.Namespace().Cache(),
			appCtx.ProjectHelmChart(),
			appCtx.ProjectHelmChart().Cache(),
			addChartDataWrapper(ctx, opts.HelmApiVersion, questionsYaml, valuesYaml, appCtx),
		)
	} else {
		projectGetter = namespace.NewSingleNamespaceProjectGetter(
			systemNamespace,
			opts.SystemNamespaces,
			appCtx.Core.Namespace(),
		)
		systemNamespaceObj, err := appCtx.Core.Namespace().Get(systemNamespace, v1.GetOptions{})
		if err != nil {
			logrus.Fatalf("unable to get systemNamespace %s", systemNamespace)
		}
		if err := addChartData(systemNamespaceObj, opts.HelmApiVersion, questionsYaml, valuesYaml, appCtx); err != nil {
			logrus.Fatal(err)
		}
	}

	project.Register(ctx,
		systemNamespace,
		opts,
		appCtx.Apply,
		appCtx.ProjectHelmChart(),
		appCtx.ProjectHelmChart().Cache(),
		appCtx.HelmController.HelmChart(),
		appCtx.HelmLocker.HelmRelease(),
		appCtx.Core.Namespace(),
		appCtx.Core.Namespace().Cache(),
		projectGetter,
	)

	release.Register(ctx,
		systemNamespace,
		appCtx.HelmLocker.HelmRelease(),
		appCtx.HelmLocker.HelmRelease().Cache(),
		appCtx.Core.Secret(),
		appCtx.Core.Secret().Cache(),
		appCtx.K8s,
		appCtx.ObjectSetRegister,
		appCtx.ObjectSetHandler,
		recorder,
	)

	chart.Register(ctx,
		systemNamespace,
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

	// must acquire all locks in order to start controllers
	leader.RunOrDie(ctx, systemNamespace, "helm-controller-lock", appCtx.K8s, func(ctx context.Context) {
		leader.RunOrDie(ctx, systemNamespace, "helm-locker-lock", appCtx.K8s, func(ctx context.Context) {
			leader.RunOrDie(ctx, systemNamespace, "helm-project-operator-lock", appCtx.K8s, func(ctx context.Context) {
				if err := appCtx.start(ctx); err != nil {
					logrus.Fatal(err)
				}
				logrus.Info("All controllers have been started")
			})
		})
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

func newContext(cfg clientcmd.ClientConfig, systemNamespace string, opts common.Options) (*appContext, error) {
	client, err := cfg.ClientConfig()
	if err != nil {
		return nil, err
	}
	client.RateLimiter = ratelimit.None

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

	// Helm Project Controller

	var namespace string // by default, this is unset so we watch everything
	if len(opts.ProjectLabel) == 0 {
		// we only need to watch the systemNamespace
		namespace = systemNamespace
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

	objectSet, objectSetRegister, objectSetHandler := objectset.NewLockableObjectSetRegister("object-set-register", apply, scf, discovery, nil)

	helmlocker, err := helmlocker.NewFactoryFromConfigWithOptions(client, &generic.FactoryOptions{
		SharedControllerFactory: scf,
		Namespace:               systemNamespace,
	})
	if err != nil {
		return nil, err
	}
	helmlockerv := helmlocker.Helm().V1alpha1()

	// Helm Controllers - should be scoped to the system namespace only

	helm, err := helm.NewFactoryFromConfigWithOptions(client, &generic.FactoryOptions{
		SharedControllerFactory: scf,
		Namespace:               systemNamespace,
	})
	if err != nil {
		return nil, err
	}
	helmv := helm.Helm().V1()

	batch, err := batch.NewFactoryFromConfigWithOptions(client, &generic.FactoryOptions{
		SharedControllerFactory: scf,
		Namespace:               systemNamespace,
	})
	if err != nil {
		return nil, err
	}
	batchv := batch.Batch().V1()

	rbac, err := rbac.NewFactoryFromConfigWithOptions(client, &generic.FactoryOptions{
		SharedControllerFactory: scf,
		Namespace:               systemNamespace,
	})
	if err != nil {
		return nil, err
	}
	rbacv := rbac.Rbac().V1()

	return &appContext{
		Interface: helmprojectv,

		K8s:  k8s,
		Core: corev,

		HelmLocker:        helmlockerv,
		ObjectSetRegister: objectSetRegister,
		ObjectSetHandler:  objectSetHandler,

		HelmController: helmv,
		Batch:          batchv,
		RBAC:           rbacv,

		Apply:            apply.WithSetOwnerReference(false, false),
		EventBroadcaster: record.NewBroadcaster(),

		ClientConfig: cfg,
		starters: []start.Starter{
			core,
			batch,
			rbac,
			helm,
			objectSet,
			helmlocker,
			helmproject,
		},
	}, nil
}
