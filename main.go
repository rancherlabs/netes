package main

import (
	"fmt"
	"os"

	"github.com/rancher/netes/master"
	"github.com/rancher/netes/store"
	"github.com/rancher/netes/types"
)

func main() {
	dsn := os.Getenv("CATTLE_DB_DSN")
	if dsn == "" {
		dsn = store.FormatDSN(
			"cattle",
			"cattle",
			"localhost:3306",
			"cattle",
			"",
		)
	}

	err := master.New(&types.GlobalConfig{
		Dialect:    "mysql",
		DSN:        dsn,
		CattleURL:  "http://localhost:8081/v3/",
		ListenAddr: ":8089",
		AdmissionControllers: []string{
			"NamespaceLifecycle",
			"LimitRanger",
			"ServiceAccount",
			"PersistentVolumeLabel",
			"DefaultStorageClass",
			"ResourceQuota",
			"DefaultTolerationSeconds",
		},
		ServiceNetCidr: "10.43.0.0/24",
	}).Run()

	fmt.Fprintf(os.Stdout, "Failed to run netes: %v", err)
	os.Exit(1)
}
