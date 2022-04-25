package health

import (
	"fmt"
	"net/http"

	"github.com/aiyengar2/helm-project-operator/pkg/controllers/common"
	"k8s.io/apiserver/pkg/server/healthz"
)

type ProbeSetter interface {
	SetReady()
}

type handler struct {
	opts common.Options

	ready bool
}

func Register(opts common.Options) ProbeSetter {
	var mux common.HTTPRequestMux
	if opts.HTTPRequestMux != nil {
		mux = opts.HTTPRequestMux
	} else {
		mux = http.DefaultServeMux
	}

	h := &handler{}
	healthz.InstallHandler(mux, h)
	return h
}

func (h *handler) Name() string {
	return fmt.Sprintf("hpo-%s-health-check", h.opts.ReleaseName)
}

func (h *handler) Check(_ *http.Request) error {
	if !h.ready {
		return fmt.Errorf("not ready")
	}
	return nil
}

func (h *handler) SetReady() {
	h.ready = true
}
