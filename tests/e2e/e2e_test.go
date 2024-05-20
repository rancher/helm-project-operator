package e2e_test

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/kralicky/kmatch"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/mod/semver"

	corev1 "k8s.io/api/core/v1"
	// "sigs.k8s.io/controller-runtime/pkg/client"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
)

const (
	// TODO : this will be subject to change

	labelProjectId = "field.cattle.io/projectId"
	annoProjectId  = "field.cattle.io/projectId"

	labelHelmProj           = "helm.cattle.io/projectId"
	labelOperatedByHelmProj = "helm.cattle.io/helm-project-operated"

	testProjectName = "p-example"
)

func projectNamespace(project string) string {
	return fmt.Sprintf("cattle-project-%s", project)
}

type helmInstaller struct {
	helmInstallOptions
}

func (h *helmInstaller) build() (*exec.Cmd, error) {
	if h.releaseName == "" {
		return nil, errors.New("helm release name must be set")
	}
	if h.chartRegistry == "" {
		return nil, errors.New("helm chart registry must be set")
	}
	args := []string{
		"upgrade",
		"--install",
	}
	if h.createNamespace {
		args = append(args, "--create-namespace")
	}
	if h.namespace != "" {
		args = append(args, "-n", h.namespace)
	}
	args = append(args, h.releaseName)
	for k, v := range h.values {
		args = append(args, "--set", fmt.Sprintf("%s=%s", k, v))
	}
	args = append(args, h.chartRegistry)
	GinkgoWriter.Print(strings.Join(append([]string{"helm"}, append(args, "\n")...), " "))
	cmd := exec.CommandContext(h.ctx, "helm", args...)
	return cmd, nil
}

func newHelmInstaller(opts ...helmInstallerOption) *helmInstaller {
	h := &helmInstaller{
		helmInstallOptions: helmInstallerDefaultOptions(),
	}
	for _, opt := range opts {
		opt(&h.helmInstallOptions)
	}
	return h

}

func helmInstallerDefaultOptions() helmInstallOptions {
	return helmInstallOptions{
		ctx:             context.Background(),
		createNamespace: false,
		namespace:       "default",
		releaseName:     "helm-project-operator",
		chartRegistry:   "https://charts.helm.sh/stable",
		values:          make(map[string]string),
	}
}

type helmInstallOptions struct {
	ctx             context.Context
	createNamespace bool
	namespace       string
	releaseName     string
	chartRegistry   string
	values          map[string]string
}

type helmInstallerOption func(*helmInstallOptions)

func WithContext(ctx context.Context) helmInstallerOption {
	return func(h *helmInstallOptions) {
		h.ctx = ctx
	}
}

func WithCreateNamespace() helmInstallerOption {
	return func(h *helmInstallOptions) {
		h.createNamespace = true
	}
}

func WithNamespace(namespace string) helmInstallerOption {
	return func(h *helmInstallOptions) {
		h.namespace = namespace
	}
}

func WithReleaseName(releaseName string) helmInstallerOption {
	return func(h *helmInstallOptions) {
		h.releaseName = releaseName
	}
}

func WithChartRegistry(chartRegistry string) helmInstallerOption {
	return func(h *helmInstallOptions) {
		h.chartRegistry = chartRegistry
	}
}

func WithValue(key string, value string) helmInstallerOption {
	return func(h *helmInstallOptions) {
		if _, ok := h.values[key]; ok {
			panic("duplicate helm value set, likely uninteded behaviour")
		}
		h.values[key] = value
	}
}

var _ = Describe("E2E helm project operator tests", Ordered, Label("integration"), func() {
	BeforeAll(func() {
		By("checking the cluster server version info")
		discoveryClient, err := discovery.NewDiscoveryClientForConfig(cfg)
		Expect(err).To(Succeed(), "Failed to create discovery client")
		serverVersion, err := discoveryClient.ServerVersion()
		Expect(err).To(Succeed(), "Failed to get server version")
		GinkgoWriter.Print(
			fmt.Sprintf("Running e2e tests against Kubernetes distribution %s %s\n",
				strings.TrimPrefix(semver.Build(serverVersion.GitVersion), "+"),
				semver.MajorMinor(serverVersion.GitVersion),
			),
		)
	})

	When("We install the helm project operator", func() {
		It("should install from the latest charts", func() {
			ctxT, ca := context.WithTimeout(testCtx, 5*time.Minute)
			defer ca()
			helmInstaller := newHelmInstaller(
				WithContext(ctxT),
				WithCreateNamespace(),
				WithNamespace("cattle-helm-system"),
				WithReleaseName("helm-project-operator"),
				WithChartRegistry("../../charts/helm-project-operator"),
				WithValue("image.repository", "rancher/helm-project-operator"),
				WithValue("image.tag", "dev"),
				WithValue("helmController.enabled", "false"),
			)
			cmd, err := helmInstaller.build()
			Expect(err).To(Succeed())
			session, err := StartCmd(cmd)
			Expect(err).To(Succeed(), "helm install command failed")
			err = session.Wait()
			Expect(err).To(Succeed(), "helm install command failed to exit successfully")
		})

		It("Should create a helm project operator deployment", func() {
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "helm-project-operator",
					Namespace: "cattle-helm-system",
				},
			}
			Eventually(Object(deploy)).Should(ExistAnd(
				HaveMatchingContainer(And(
					HaveName("helm-project-operator"),
					HaveImage("rancher/helm-project-operator:dev"),
				)),
			))

			Eventually(
				Object(deploy),
				time.Second*90, time.Millisecond*333,
			).Should(HaveSuccessfulRollout())

		})

		Context("Check that project registration namespace is created", func() {
			It("Should create a project registration namespace", func() {
				By("creating the project registration namespace")
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "e2e-hpo",
						Labels: map[string]string{
							// Note : this will be rejected by webhook if rancher/rancher is managing this cluster
							labelProjectId: "p-example",
						},
						Annotations: map[string]string{
							annoProjectId: fmt.Sprintf("local:%s", testProjectName),
						},
					},
				}
				err := k8sClient.Create(testCtx, ns)
				exists := apierrors.IsAlreadyExists(err)
				if !exists {
					Expect(err).To(Succeed(), "Failed to create project registration namespace")
				}
				Eventually(Object(ns)).Should(Exist())

				By("verifying the helm project namespace has been create by the controller")
				projNs := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: projectNamespace(testProjectName),
					},
				}
				Eventually(Object(projNs), 60*time.Second).Should(ExistAnd(
					HaveLabels(
						labelProjectId, testProjectName,
						labelOperatedByHelmProj, "true",
						labelHelmProj, testProjectName,
					),
					HaveAnnotations(
						labelProjectId, testProjectName,
					),
				))
			})
		})
	})
})
