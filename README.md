helm-project-operator
========

The Helm Project Operator is a generic design for a Kubernetes Operator that acts on `ProjectHelmChart` CRs.

**Note: this project is not intended for standalone use.** 

It is intended to be implemented by a Project Operator (e.g. `rancher/prometheus-federator`) but provides a common definition for all Project Operators to use in order to support deploy specific, pre-bundled Helm charts (tied to a unique registered `spec.helmApiVersion` associated with the operator) across all project namespaces detected by this operator.

## What is a Project Helm Chart

TBD

## What is a Project Operator?

TBD

## Building

`make`

## Running

`./bin/helm-project-operator`

## License
Copyright (c) 2020 [Rancher Labs, Inc.](http://rancher.com)

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

[http://www.apache.org/licenses/LICENSE-2.0](http://www.apache.org/licenses/LICENSE-2.0)

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
