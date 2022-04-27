package crd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	helmlockercrd "github.com/aiyengar2/helm-locker/pkg/crd"
	v1alpha1 "github.com/aiyengar2/helm-project-operator/pkg/apis/helm.cattle.io/v1alpha1"
	helmcontrollercrd "github.com/k3s-io/helm-controller/pkg/crd"
	"github.com/rancher/wrangler/pkg/crd"
	"github.com/rancher/wrangler/pkg/yaml"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

func WriteFiles(crdDirpath, crdDepDirpath string) error {
	objs, depObjs, err := Objects(false)
	if err != nil {
		return err
	}
	if err := writeFiles(crdDirpath, objs); err != nil {
		return err
	}
	if err := writeFiles(crdDepDirpath, depObjs); err != nil {
		return err
	}
	return nil
}

func writeFiles(dirpath string, objs []runtime.Object) error {
	if err := os.MkdirAll(dirpath, 0755); err != nil {
		return err
	}

	objMap := make(map[string][]byte)

	for _, o := range objs {
		data, err := yaml.Export(o)
		if err != nil {
			return err
		}
		meta, err := meta.Accessor(o)
		if err != nil {
			return err
		}
		key := strings.SplitN(meta.GetName(), ".", 2)[0]
		objMap[key] = data
	}

	var wg sync.WaitGroup
	wg.Add(len(objMap))
	for key, data := range objMap {
		go func(key string, data []byte) {
			defer wg.Done()
			f, err := os.Create(filepath.Join(dirpath, fmt.Sprintf("crd-%s.yaml", key)))
			if err != nil {
				logrus.Error(err)
			}
			defer f.Close()
			_, err = f.Write(data)
			if err != nil {
				logrus.Error(err)
			}
		}(key, data)
	}
	wg.Wait()

	return nil
}

func Print(out io.Writer, depOut io.Writer) {
	objs, depObjs, err := Objects(false)
	if err != nil {
		logrus.Fatalf("%s", err)
	}
	objsV1Beta1, depObjsV1Beta1, err := Objects(true)
	if err != nil {
		logrus.Fatalf("%s", err)
	}
	if err := print(out, objs, objsV1Beta1); err != nil {
		logrus.Fatalf("%s", err)
	}
	if err := print(depOut, depObjs, depObjsV1Beta1); err != nil {
		logrus.Fatalf("%s", err)
	}
}

func print(out io.Writer, objs []runtime.Object, objsV1Beta1 []runtime.Object) error {
	data, err := yaml.Export(objs...)
	if err != nil {
		return err
	}
	dataV1Beta1, err := yaml.Export(objsV1Beta1...)
	if err != nil {
		return err
	}
	data = append([]byte("{{- if .Capabilities.APIVersions.Has \"apiextensions.k8s.io/v1\" -}}\n"), data...)
	data = append(data, []byte("{{- else -}}\n---\n")...)
	data = append(data, dataV1Beta1...)
	data = append(data, []byte("{{- end -}}")...)
	_, err = out.Write(data)
	return err
}

func Objects(v1beta1 bool) (crds, crdDeps []runtime.Object, err error) {
	crdDefs, crdDepDefs := List()
	crds, err = objects(v1beta1, crdDefs)
	if err != nil {
		return nil, nil, err
	}
	crdDeps, err = objects(v1beta1, crdDepDefs)
	if err != nil {
		return nil, nil, err
	}
	return
}

func objects(v1beta1 bool, crdDefs []crd.CRD) (crds []runtime.Object, err error) {
	for _, crdDef := range crdDefs {
		if v1beta1 {
			crd, err := crdDef.ToCustomResourceDefinitionV1Beta1()
			if err != nil {
				return nil, err
			}
			crds = append(crds, crd)
		} else {
			crd, err := crdDef.ToCustomResourceDefinition()
			if err != nil {
				return nil, err
			}
			crds = append(crds, crd)
		}
	}
	return
}

func List() ([]crd.CRD, []crd.CRD) {
	crds := []crd.CRD{
		newCRD(&v1alpha1.ProjectHelmChart{}, func(c crd.CRD) crd.CRD {
			return c.
				WithColumn("Status", ".status.status").
				WithColumn("System Namespace", ".status.systemNamespace").
				WithColumn("Release Namespace", ".status.releaseNamespace").
				WithColumn("Release Name", ".status.releaseName").
				WithColumn("Target Namespaces", ".status.targetNamespaces")
		}),
	}
	crdDeps := append(helmcontrollercrd.List(), helmlockercrd.List()...)
	return crds, crdDeps
}

func Create(ctx context.Context, cfg *rest.Config) error {
	factory, err := crd.NewFactoryFromClient(cfg)
	if err != nil {
		return err
	}

	crds, crdDeps := List()
	return factory.BatchCreateCRDs(ctx, append(crds, crdDeps...)...).BatchWait()
}

func newCRD(obj interface{}, customize func(crd.CRD) crd.CRD) crd.CRD {
	crd := crd.CRD{
		GVK: schema.GroupVersionKind{
			Group:   "helm.cattle.io",
			Version: "v1alpha1",
		},
		Status:       true,
		SchemaObject: obj,
	}
	if customize != nil {
		crd = customize(crd)
	}
	return crd
}
