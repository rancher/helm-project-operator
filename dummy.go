package main

import (
	_ "embed"
	"log"
	"net/http"
	_ "net/http/pprof"

	"github.com/aiyengar2/helm-project-operator/pkg/controllers/common"
	"github.com/aiyengar2/helm-project-operator/pkg/operator"
	"github.com/aiyengar2/helm-project-operator/pkg/version"
	command "github.com/rancher/wrangler-cli"
	_ "github.com/rancher/wrangler/pkg/generated/controllers/apiextensions.k8s.io"
	_ "github.com/rancher/wrangler/pkg/generated/controllers/networking.k8s.io"
	"github.com/rancher/wrangler/pkg/kubeconfig"
	"github.com/spf13/cobra"
)

const (
	DummyHelmApiVersion = "dummy.cattle.io/v1alpha1"
)

var (
	DummySystemNamespaces = []string{"kube-system"}

	//go:embed bin/example-chart/example-chart.tgz.base64
	base64TgzChart string

	debugConfig command.DebugConfig
)

type DummyType map[string]interface{}

type DummyOperator struct {
	Kubeconfig string `usage:"Kubeconfig file"`
	Namespace  string `usage:"Namespace to watch for ProjectHelmCharts; this will be ignored if project label is provided" env:"NAMESPACE"`
	NodeName   string `usage:"Name of the node this controller is running on" env:"NODE_NAME"`

	ProjectLabel            string `usage:"Label on namespaces to create Project Registration Namespaces and watch for ProjectHelmCharts" env:"PROJECT_LABEL"`
	SystemProjectLabelValue string `usage:"Value on project label on namespaces that marks it as a system namespace" env:"SYSTEM_PROJECT_LABEL_VALUE"`
}

func (o *DummyOperator) Run(cmd *cobra.Command, args []string) error {
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()
	debugConfig.MustSetupDebug()

	cfg := kubeconfig.GetNonInteractiveClientConfig(o.Kubeconfig)

	ctx := cmd.Context()

	if err := operator.Init(ctx, o.Namespace, cfg, common.Options{
		HelmApiVersion:   DummyHelmApiVersion,
		ValuesType:       DummyType{},
		SystemNamespaces: DummySystemNamespaces,
		ChartContent:     base64TgzChart,

		ProjectLabel:            o.ProjectLabel,
		SystemProjectLabelValue: o.SystemProjectLabelValue,

		NodeName: o.NodeName,
	}); err != nil {
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
