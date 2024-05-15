#!/bin/bash
set -e

helm repo add rancher-stable https://releases.rancher.com/server-charts/stable

helm repo update

kubectl create namespace cattle-system

helm install -n cattle-system rancher rancher-stable/rancher