#!/bin/bash
set -e

source $(dirname $0)/entry

cd $(dirname $0)/../../../..

kubectl create namespace e2e-hpo || true
kubectl annotate namespace e2e-hpo field.cattle.io/projectId=local:p-example --overwrite
sleep "${DEFAULT_SLEEP_TIMEOUT_SECONDS}"
if ! kubectl get namespace cattle-project-p-example; then
    echo "DEBUG"
    kubectl get ns
    echo "ERROR: Expected cattle-project-p-example namespace to exist after ${DEFAULT_SLEEP_TIMEOUT_SECONDS} seconds, not found"
    exit 1
fi

echo "PASS: Project Registration Namespace was created"
