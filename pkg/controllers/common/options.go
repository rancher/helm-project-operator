package common

import (
	"errors"
	"fmt"

	"github.com/sirupsen/logrus"
)

// Options defines options that can be set on initializing the HelmProjectOperator
type Options struct {
	HelmApiVersion   string
	ValuesType       interface{}
	SystemNamespaces []string
	ChartContent     string

	ProjectLabel    string
	SystemProjectID string

	HelmJobImage string
	NodeName     string
}

func (opts Options) Validate() error {
	if len(opts.HelmApiVersion) == 0 {
		return errors.New("must provide a spec.helmApiVersion that this project operator is being initialized for")
	}
	if opts.ValuesType == nil {
		return fmt.Errorf("must provide a type for the values.yaml spec that can be used to validate spec.values on ProjectHelmCharts with spec.helmApiVersion=%s", opts.HelmApiVersion)
	}

	if len(opts.SystemNamespaces) > 0 {
		logrus.Infof("Marking the following namespaces as system namespaces: %s", opts.SystemNamespaces)
	}

	if len(opts.ProjectLabel) > 0 {
		logrus.Infof("Creating dedicated project system namespaces based on the value found for the project label %s on all namespaces in the cluster, excluding system namespaces; these namespaces will need to be manually cleaned up", opts.ProjectLabel)
		if len(opts.SystemProjectID) > 0 {
			logrus.Infof("Assuming namespaces tagged with %s=%s are also system namespaces; this label will also be added on all dedicated project system namespaces created by this operator", opts.ProjectLabel, opts.SystemProjectID)
		}
	}
	return nil
}
