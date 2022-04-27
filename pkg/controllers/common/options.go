package common

import (
	"errors"

	"github.com/sirupsen/logrus"
)

// Options defines options that can be set on initializing the HelmProjectOperator
type Options struct {
	RuntimeOptions
	OperatorOptions
}

// Validate validates the provided Options
func (opts Options) Validate() error {
	if err := opts.OperatorOptions.Validate(); err != nil {
		return err
	}

	if err := opts.RuntimeOptions.Validate(); err != nil {
		return err
	}

	// Cross option checks

	if opts.Singleton {
		logrus.Infof("Note: Operator only supports a single ProjectHelmChart per project registration namespace")
		if len(opts.ProjectLabel) == 0 {
			logrus.Warnf("It is only recommended to run a singleton Project Operator when --project-label is provided (currently not set). The current configuration of this operator would only allow a single ProjectHelmChart to be managed by this Operator.")
		}
	}

	for subjectRole, defaultClusterRoleName := range GetDefaultClusterRoles(opts) {
		logrus.Infof("RoleBindings will automatically be created for Roles in the Project Release Namespace marked with '%s': '<helm-release>' "+
			"and '%s': '%s' based on ClusterRoleBindings or RoleBindings in the Project Registration namespace tied to ClusterRole %s",
			HelmProjectOperatorProjectHelmChartRoleLabel, HelmProjectOperatorProjectHelmChartRoleAggregateFromLabel, subjectRole, defaultClusterRoleName,
		)
	}

	return nil
}

// OperatorOptions are options provided by an operator that is implementing Helm Project Operator
type OperatorOptions struct {
	// HelmApiVersion is the unique API version marking ProjectHelmCharts that this Helm Project Operator should watch for
	HelmApiVersion string

	// ReleaseName is a name that identifies releases created for this operator
	ReleaseName string

	// SystemNamespaces are additional operator namespaces to treat as if they are system namespaces whether or not
	// they are marked via some sort of annotation
	SystemNamespaces []string

	// ChartContent is the base64 tgz contents of the folder containing the Helm chart that needs to be deployed
	ChartContent string

	// Singleton marks whether only a single ProjectHelmChart can exist per registration namespace
	// If enabled, it will ensure that releases are named based on the registration namespace rather than
	// the name provided on the ProjectHelmChart, which is what triggers an UnableToCreateHelmRelease status
	// on the ProjectHelmChart created after this one
	Singleton bool
}

// Validate validates the provided OperatorOptions
func (opts OperatorOptions) Validate() error {
	if len(opts.HelmApiVersion) == 0 {
		return errors.New("must provide a spec.helmApiVersion that this project operator is being initialized for")
	}

	if len(opts.ReleaseName) == 0 {
		return errors.New("must provide name of Helm release that this project operator should deploy")
	}

	if len(opts.SystemNamespaces) > 0 {
		logrus.Infof("Marking the following namespaces as system namespaces: %s", opts.SystemNamespaces)
	}

	if len(opts.ChartContent) == 0 {
		return errors.New("cannot instantiate Project Operator without bundling a Helm chart to provide for the HelmChart's spec.ChartContent")
	}

	return nil
}
