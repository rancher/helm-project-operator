package projectoperator

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/rancher/helm-project-operator/pkg/controllers/common"
	"k8s.io/client-go/tools/clientcmd"
)

type ProjectOperator struct {
	ctx             context.Context
	systemNamespace string
	clientConfig    clientcmd.ClientConfig
	opts            common.Options

	ValuesYaml    string
	QuestionsYaml string
}

func NewProjectOperator(
	ctx context.Context,
	systemNamespace string,
	clientConfig clientcmd.ClientConfig,
	opts common.Options,
) (*ProjectOperator, error) {
	// always add the systemNamespace to the systemNamespaces provided
	opts.SystemNamespaces = append(opts.SystemNamespaces, systemNamespace)

	if len(opts.ControllerName) == 0 {
		opts.ControllerName = "helm-project-operator"
	}

	p := &ProjectOperator{
		ctx:             ctx,
		systemNamespace: systemNamespace,
		clientConfig:    clientConfig,
		opts:            opts,
	}

	valuesYaml, questionsYaml, err := parseValuesAndQuestions(p.opts.ChartContent)
	if err != nil {
		return nil, err
	}
	p.ValuesYaml = valuesYaml
	p.QuestionsYaml = questionsYaml

	if err := p.Validate(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p ProjectOperator) Context() context.Context {
	return p.ctx
}

func (p ProjectOperator) Options() common.Options {
	return p.opts
}

// The system namespace of the project operator
func (p ProjectOperator) Namespace() string {
	return p.systemNamespace
}

func (p ProjectOperator) ClientConfig() clientcmd.ClientConfig {
	return p.clientConfig
}

func (p ProjectOperator) Validate() error {
	if p.systemNamespace == "" {
		return fmt.Errorf("system namespace was not specified, unclear where to place HelmCharts or HelmReleases")
	}

	if p.ValuesYaml == "" {
		return fmt.Errorf("values.yaml was not found in the base64TgzChart provided")
	}
	if p.QuestionsYaml == "" {
		return fmt.Errorf("questions.yaml was not found in the base64TgzChart provided")
	}

	if err := p.opts.Validate(); err != nil {
		return err
	}

	return nil
}

// parseValuesAndQuestions parses the base64TgzChart and emits the values.yaml and questions.yaml contained within it
// If values.yaml or questions.yaml are not specified, it will return an empty string for each
func parseValuesAndQuestions(base64TgzChart string) (string, string, error) {
	tgzChartBytes, err := base64.StdEncoding.DecodeString(base64TgzChart)
	if err != nil {
		return "", "", fmt.Errorf("unable to decode base64TgzChart to tgzChart: %s", err)
	}
	gzipReader, err := gzip.NewReader(bytes.NewReader(tgzChartBytes))
	if err != nil {
		return "", "", fmt.Errorf("unable to create gzipReader to read from base64TgzChart: %s", err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	var valuesYamlBuffer, questionsYamlBuffer bytes.Buffer
	var foundValuesYaml, foundQuestionsYaml bool
	for {
		h, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", "", err
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		splitName := strings.SplitN(h.Name, string(os.PathSeparator), 2)
		nameWithoutRootDir := splitName[0]
		if len(splitName) > 1 {
			nameWithoutRootDir = splitName[1]
		}
		if nameWithoutRootDir == "values.yaml" || nameWithoutRootDir == "values.yml" {
			if foundValuesYaml {
				// multiple values.yaml
				return "", "", errors.New("multiple values.yaml or values.yml found in base64TgzChart provided")
			}
			foundValuesYaml = true
			io.Copy(&valuesYamlBuffer, tarReader)
		}
		if nameWithoutRootDir == "questions.yaml" || nameWithoutRootDir == "questions.yml" {
			if foundQuestionsYaml {
				// multiple values.yaml
				return "", "", errors.New("multiple questions.yaml or questions.yml found in base64TgzChart provided")
			}
			foundQuestionsYaml = true
			io.Copy(&questionsYamlBuffer, tarReader)
		}
	}
	return valuesYamlBuffer.String(), questionsYamlBuffer.String(), nil
}
