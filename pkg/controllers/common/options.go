package common

import (
	"errors"

	"github.com/sirupsen/logrus"
)

// Options defines options that can be set on initializing the HelmProjectOperator
type Options struct {
	HelmApiVersion   string
	SystemNamespaces []string
	ChartContent     string

	ProjectLabel            string
	SystemProjectLabelValue string

	HelmJobImage string
	NodeName     string
}

func (opts Options) Validate() error {
	if len(opts.HelmApiVersion) == 0 {
		return errors.New("must provide a spec.helmApiVersion that this project operator is being initialized for")
	}

	if len(opts.SystemNamespaces) > 0 {
		logrus.Infof("marking the following namespaces as system namespaces: %s", opts.SystemNamespaces)
	}

	if len(opts.ChartContent) == 0 {
		return errors.New("cannot instantiate Project Operator without bundling a Helm chart to provide for the HelmChart's spec.ChartContent")
	}

	if len(opts.ProjectLabel) > 0 {
		logrus.Infof("creating dedicated project registration namespaces to discover ProjectHelmCharts based on the value found for the project label %s on all namespaces in the cluster, excluding system namespaces; these namespaces will need to be manually cleaned up if they have the label '%s: \"true\"'", opts.ProjectLabel, HelmProjectOperatedOrphanedLabel)
		if len(opts.SystemProjectLabelValue) > 0 {
			logrus.Infof("assuming namespaces tagged with %s=%s are also system namespaces", opts.ProjectLabel, opts.SystemProjectLabelValue)
		}
	}

	if len(opts.HelmJobImage) > 0 {
		logrus.Infof("using %s as spec.JobImage on all generated HelmChart resources", opts.HelmJobImage)
	}

	if len(opts.NodeName) > 0 {
		logrus.Infof("marking events as being sourced from node %s", opts.NodeName)
	}
	return nil
}
