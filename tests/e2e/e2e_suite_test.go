package e2e_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"

	// "sigs.k8s.io/controller-runtime/pkg/client"

	env "github.com/caarlos0/env/v11"
	"github.com/kralicky/kmatch"
	dockerparser "github.com/novln/docker-parser"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

func TestE2e(t *testing.T) {
	SetDefaultEventuallyTimeout(60 * time.Second)
	SetDefaultEventuallyPollingInterval(50 * time.Millisecond)
	SetDefaultConsistentlyDuration(1 * time.Second)
	SetDefaultConsistentlyPollingInterval(50 * time.Millisecond)
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2e Suite")
}

var (
	k8sClient client.Client
	cfg       *rest.Config
	testCtx   context.Context
	clientSet *kubernetes.Clientset
)

type TestSpec struct {
	Kubeconfig string `env:"KUBECONFIG,required"`
	HpoImage   string `env:"IMAGE,required"`
}

func (t *TestSpec) Validate() error {
	var errs []error
	if _, err := dockerparser.Parse(t.HpoImage); err != nil {
		errs = append(errs, err)
	}
	if _, err := os.Stat(t.Kubeconfig); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

var _ = BeforeSuite(func() {
	ts := TestSpec{}
	Expect(env.Parse(&ts)).To(Succeed(), "Could not parse test spec from environment variables")
	Expect(ts.Validate()).To(Succeed(), "Invalid input e2e test spec")

	ctxCa, ca := context.WithCancel(context.Background())
	DeferCleanup(func() {
		ca()
	})

	testCtx = ctxCa
	newCfg, err := config.GetConfig()
	Expect(err).NotTo(HaveOccurred(), "Could not initialize kubernetes client config")
	cfg = newCfg
	newClientset, err := kubernetes.NewForConfig(cfg)
	Expect(err).To(Succeed(), "Could not initialize kubernetes clientset")
	clientSet = newClientset

	newK8sClient, err := client.New(cfg, client.Options{})
	Expect(err).NotTo(HaveOccurred(), "Could not initialize kubernetes client")
	k8sClient = newK8sClient
	kmatch.SetDefaultObjectClient(k8sClient)
})
