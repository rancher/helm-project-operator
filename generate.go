//go:generate go run pkg/helm-locker/codegen/cleanup/main.go
//go:generate go run pkg/helm-locker/codegen/main.go
//go:generate go run ./pkg/helm-locker/codegen crds ./crds/helm-locker/crds.yaml

//go:generate go run pkg/codegen/cleanup/main.go
//go:generate go run pkg/codegen/main.go
//go:generate go run ./pkg/codegen crds ./crds/helm-project-operator ./crds/helm-project-operator

package main
