package store

import (
	goml "github.com/rancher/goml-storage"
	"github.com/rancher/netes/types"
	"k8s.io/apimachinery/pkg/runtime/schema"
	serverstorage "k8s.io/apiserver/pkg/server/storage"
	"k8s.io/apiserver/pkg/storage/storagebackend"
	"k8s.io/apiserver/pkg/storage/storagebackend/factory"
	"k8s.io/apiserver/pkg/util/flag"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/apis/batch"
	"k8s.io/kubernetes/pkg/kubeapiserver"
	"k8s.io/kubernetes/pkg/master"
)

const StorageTypeRDBMS = "mysql"

func init() {
	factory.Register(StorageTypeRDBMS, goml.NewRDBMSStorage)
}

func StorageFactory(pathPrefix string, config *types.GlobalConfig) (*serverstorage.DefaultStorageFactory, error) {
	storageConfig := storagebackend.NewDefaultConfig(pathPrefix, api.Scheme, nil)
	storageConfig.Type = StorageTypeRDBMS
	storageConfig.ServerList = []string{
		config.Dialect,
		config.DSN,
	}

	return kubeapiserver.NewStorageFactory(
		*storageConfig,
		//"application/vnd.kubernetes.protobuf",
		"application/json",
		api.Codecs,
		serverstorage.NewDefaultResourceEncodingConfig(api.Registry),
		nil,
		// TODO: Needed?
		[]schema.GroupVersionResource{
			batch.Resource("cronjobs").WithVersion("v2alpha1"),
		},
		master.DefaultAPIResourceConfigSource(),
		flag.ConfigurationMap{
			"batch/v2alpha1": "true",
		})
}
