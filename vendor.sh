#!/bin/bash
set -e

trash

rm -rf vendor/github.com/rancher/goml-storage
git checkout vendor/github.com/rancher/goml-storage
rm -rf vendor/github.com/rancher/go-rancher
git checkout vendor/github.com/rancher/go-rancher
rm -rf vendor/k8s.io/apiserver
git checkout vendor/k8s.io/apiserver
