package common

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
)

type HardeningOptions struct {
	ServiceAccount *DefaultServiceAccountOptions `yaml:"serviceAccountSpec"`
	NetworkPolicy  *DefaultNetworkPolicyOptions  `yaml:"networkPolicySpec"`
}

type DefaultServiceAccountOptions struct {
	Secrets                      []corev1.ObjectReference      `yaml:"secrets,omitempty"`
	ImagePullSecrets             []corev1.LocalObjectReference `yaml:"imagePullSecrets,omitempty"`
	AutomountServiceAccountToken *bool                         `yaml:"automountServiceAccountToken,omitEmpty"`
}

type DefaultNetworkPolicyOptions networkingv1.NetworkPolicySpec

// LoadHardeningOptionsFromFile unmarshalls the struct found at the file to YAML and reads it into memory
func LoadHardeningOptionsFromFile(path string) (HardeningOptions, error) {
	var hardeningOptions HardeningOptions
	wd, err := os.Getwd()
	if err != nil {
		return HardeningOptions{}, err
	}
	abspath := filepath.Join(wd, path)
	_, err = os.Stat(abspath)
	if err != nil {
		if os.IsNotExist(err) {
			// we just assume the default is used
			err = nil
		}
		return HardeningOptions{}, err
	}
	hardeningOptionsBytes, err := ioutil.ReadFile(abspath)
	if err != nil {
		return hardeningOptions, err
	}
	return hardeningOptions, yaml.UnmarshalStrict(hardeningOptionsBytes, &hardeningOptions)
}
