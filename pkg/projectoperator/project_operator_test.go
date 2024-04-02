package projectoperator_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/helm-project-operator/pkg/controllers/common"
	"github.com/rancher/helm-project-operator/pkg/projectoperator"
	"github.com/rancher/helm-project-operator/pkg/testdata"
)

var (
	// DummyHelmAPIVersion is the spec.helmApiVersion corresponding to the dummy example-chart
	dummyHelmAPIVersion = "dummy.cattle.io/v1alpha1"

	// DummyReleaseName is the release name corresponding to the operator that deploys the dummy example-chart
	dummyReleaseName      = "dummy"
	dummySystemNamespaces = []string{"kube-system"}
)

var _ = Describe("ProjectOperator", func() {
	When("We create a new project operator from a valid helm chart", func() {
		It("should successfully create a project operator from defaultish values", func() {
			exampleOperator, err := projectoperator.NewProjectOperator(
				context.Background(),
				"example-p",
				nil,
				common.Options{
					OperatorOptions: common.OperatorOptions{
						HelmAPIVersion:   dummyHelmAPIVersion,
						ReleaseName:      dummyReleaseName,
						SystemNamespaces: dummySystemNamespaces,
						ChartContent:     string(testdata.TestData("charts/example-chart.tgz.base64")),
						Singleton:        false,
					},
					RuntimeOptions: common.RuntimeOptions{},
				},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(exampleOperator).NotTo(BeNil())
			Expect(exampleOperator.Context()).NotTo(BeNil())
			Expect(exampleOperator.Namespace()).To(Equal("example-p"))
			Expect(exampleOperator.Options().ControllerName).To(Equal("helm-project-operator"))
			Expect(exampleOperator.Options().SystemNamespaces).To(ContainElement("example-p"))
			Expect(exampleOperator.Options().HelmAPIVersion).To(Equal(dummyHelmAPIVersion))
			Expect(exampleOperator.Options().ReleaseName).To(Equal(dummyReleaseName))
			Expect(exampleOperator.ValuesYaml).NotTo(BeEmpty())
			Expect(exampleOperator.QuestionsYaml).NotTo(BeEmpty())
			// Expect(exampleOperator.Options)
		})
	})

	When("We create a new project operator from an invalid configuration", func() {
		It("should fail to create a project operator with an invalid helm chart", func() {
			_, err := projectoperator.NewProjectOperator(
				context.Background(),
				"example-p",
				nil,
				common.Options{
					OperatorOptions: common.OperatorOptions{
						HelmAPIVersion:   dummyHelmAPIVersion,
						ReleaseName:      dummyReleaseName,
						SystemNamespaces: dummySystemNamespaces,
						ChartContent:     "invalid-base64-chart",
						Singleton:        false,
					},
					RuntimeOptions: common.RuntimeOptions{},
				},
			)
			Expect(err).To(HaveOccurred())
		})

		It("should fail to create a project operator with an invalid system namespace", func() {
			_, err := projectoperator.NewProjectOperator(
				context.Background(),
				"",
				nil,
				common.Options{
					OperatorOptions: common.OperatorOptions{
						HelmAPIVersion:   dummyHelmAPIVersion,
						ReleaseName:      dummyReleaseName,
						SystemNamespaces: dummySystemNamespaces,
						ChartContent:     "invalid-base64-chart",
						Singleton:        false,
					},
					RuntimeOptions: common.RuntimeOptions{},
				},
			)
			Expect(err).To(HaveOccurred())
		})
	})
})
