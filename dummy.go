package main

import (
	_ "embed"
	"log"
	"net/http"
	_ "net/http/pprof"

	"github.com/rancher/helm-project-operator/pkg/controllers/common"
	"github.com/rancher/helm-project-operator/pkg/operator"
	"github.com/rancher/helm-project-operator/pkg/projectoperator"
	"github.com/rancher/helm-project-operator/pkg/version"
	command "github.com/rancher/wrangler-cli"
	_ "github.com/rancher/wrangler/pkg/generated/controllers/apiextensions.k8s.io"
	_ "github.com/rancher/wrangler/pkg/generated/controllers/networking.k8s.io"
	"github.com/rancher/wrangler/pkg/kubeconfig"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

const (
	// DummyHelmAPIVersion is the spec.helmApiVersion corresponding to the dummy example-chart
	DummyHelmAPIVersion = "dummy.cattle.io/v1alpha1"

	// DummyReleaseName is the release name corresponding to the operator that deploys the dummy example-chart
	DummyReleaseName = "dummy"
)

var (
	// DummySystemNamespaces is the system namespaces scoped for the dummy example-chart.
	DummySystemNamespaces = []string{"kube-system"}

	//go:embed bin/example-chart/example-chart.tgz.base64
	base64TgzChart string

	debugConfig command.DebugConfig
)

type DummyOperator struct {
	// Note: all Project Operator are expected to provide these RuntimeOptions
	common.RuntimeOptions

	Kubeconfig string `usage:"Kubeconfig file"`
}

func (o *DummyOperator) Run(cmd *cobra.Command, _ []string) error {
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()
	debugConfig.MustSetupDebug()

	cfg := kubeconfig.GetNonInteractiveClientConfig(o.Kubeconfig)

	ctx := cmd.Context()

	dummyOperator, err := projectoperator.NewProjectOperator(ctx, o.Namespace, cfg, common.Options{
		OperatorOptions: common.OperatorOptions{
			HelmAPIVersion:   DummyHelmAPIVersion,
			ReleaseName:      DummyReleaseName,
			SystemNamespaces: DummySystemNamespaces,
			ChartContent:     base64TgzChart,
			Singleton:        false,
		},
		RuntimeOptions: o.RuntimeOptions,
	})

	if err != nil {
		logrus.Fatal(err)
	}

	if err := operator.Init(
		*dummyOperator,
	); err != nil {
		return err
	}

	<-cmd.Context().Done()
	return nil
}

func main() {
	cmd := command.Command(&DummyOperator{}, cobra.Command{
		Version: version.FriendlyVersion(),
	})
	cmd = command.AddDebug(cmd, &debugConfig)
	command.Main(cmd)
}
