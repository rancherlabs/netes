package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_library",
)

go_library(
    name = "go_default_library",
    srcs = [
        "client.go",
        "model.go",
    ],
    tags = ["automanaged"],
    deps = [
        "//vendor/github.com/jinzhu/gorm:go_default_library",
        "//vendor/golang.org/x/net/context:go_default_library",
        "//vendor/k8s.io/apiserver/pkg/storage/kv:go_default_library",
    ],
)
