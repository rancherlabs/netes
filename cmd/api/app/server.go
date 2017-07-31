/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package app does all of the work necessary to create a Kubernetes
// APIServer by binding together the API, master and APIServer infrastructure.
// It can be configured and called directly or via the hyperkube framework.
package app

import (
	"fmt"
	"net"
	"time"

	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilnet "k8s.io/apimachinery/pkg/util/net"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/admission/initializer"
	"k8s.io/apiserver/pkg/server"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/apiserver/pkg/server/filters"
	genericoptions "k8s.io/apiserver/pkg/server/options"
	serverstorage "k8s.io/apiserver/pkg/server/storage"
	"k8s.io/apiserver/pkg/storage/storagebackend"
	clientgoclientset "k8s.io/client-go/kubernetes"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/apis/batch"
	"k8s.io/kubernetes/pkg/capabilities"
	"k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"
	informers "k8s.io/kubernetes/pkg/client/informers/informers_generated/internalversion"
	"k8s.io/kubernetes/pkg/kubeapiserver"
	kubeapiserveradmission "k8s.io/kubernetes/pkg/kubeapiserver/admission"
	kubeoptions "k8s.io/kubernetes/pkg/kubeapiserver/options"
	kubeletclient "k8s.io/kubernetes/pkg/kubelet/client"
	"k8s.io/kubernetes/pkg/master"
	"k8s.io/kubernetes/pkg/master/ports"
	quotainstall "k8s.io/kubernetes/pkg/quota/install"
	"k8s.io/kubernetes/pkg/version"
	"k8s.io/kubernetes/plugin/pkg/admission/admit"
	"k8s.io/kubernetes/plugin/pkg/admission/alwayspullimages"
	"k8s.io/kubernetes/plugin/pkg/admission/antiaffinity"
	"k8s.io/kubernetes/plugin/pkg/admission/defaulttolerationseconds"
	"k8s.io/kubernetes/plugin/pkg/admission/deny"
	"k8s.io/kubernetes/plugin/pkg/admission/exec"
	"k8s.io/kubernetes/plugin/pkg/admission/gc"
	"k8s.io/kubernetes/plugin/pkg/admission/imagepolicy"
	"k8s.io/kubernetes/plugin/pkg/admission/initialization"
	"k8s.io/kubernetes/plugin/pkg/admission/initialresources"
	"k8s.io/kubernetes/plugin/pkg/admission/limitranger"
	"k8s.io/kubernetes/plugin/pkg/admission/namespace/autoprovision"
	"k8s.io/kubernetes/plugin/pkg/admission/namespace/exists"
	"k8s.io/kubernetes/plugin/pkg/admission/noderestriction"
	"k8s.io/kubernetes/plugin/pkg/admission/persistentvolume/label"
	"k8s.io/kubernetes/plugin/pkg/admission/podnodeselector"
	"k8s.io/kubernetes/plugin/pkg/admission/podpreset"
	"k8s.io/kubernetes/plugin/pkg/admission/podtolerationrestriction"
	"k8s.io/kubernetes/plugin/pkg/admission/resourcequota"
	"k8s.io/kubernetes/plugin/pkg/admission/security/podsecuritypolicy"
	"k8s.io/kubernetes/plugin/pkg/admission/securitycontext/scdeny"
	"k8s.io/kubernetes/plugin/pkg/admission/serviceaccount"
	"k8s.io/kubernetes/plugin/pkg/admission/storageclass/setdefault"
	"k8s.io/kubernetes/plugin/pkg/admission/webhook"
	"k8s.io/apiserver/pkg/util/flag"
)

// Run runs the specified APIServer.  This should never exit.
func Run(stopCh <-chan struct{}) error {
	// To help debugging, immediately log version
	glog.Infof("Version: %+v", version.Get())

	server, err := CreateServerChain(stopCh)
	if err != nil {
		return err
	}

	return server.PrepareRun().Run(stopCh)
}

// CreateServerChain creates the apiservers connected via delegation.
func CreateServerChain(stopCh <-chan struct{}) (*genericapiserver.GenericAPIServer, error) {
	storageConfig := storagebackend.NewDefaultConfig(kubeoptions.DefaultEtcdPathPrefix, api.Scheme, nil)
	storageConfig.Type = "etcd3"
	storageConfig.ServerList = []string{"localhost"}

	etcdOptions := genericoptions.NewEtcdOptions(storageConfig)
	kubeAPIServerConfig, sharedInformers, err := CreateKubeAPIServerConfig(*etcdOptions)
	if err != nil {
		return nil, err
	}

	// TPRs are enabled and not yet beta, since this these are the successor, they fall under the same enablement rule
	// If additional API servers are added, they should be gated.
	apiExtensionsConfig, err := createAPIExtensionsConfig(*kubeAPIServerConfig.GenericConfig, *etcdOptions)
	if err != nil {
		return nil, err
	}
	apiExtensionsServer, err := createAPIExtensionsServer(apiExtensionsConfig, genericapiserver.EmptyDelegate)
	if err != nil {
		return nil, err
	}

	kubeAPIServer, err := CreateKubeAPIServer(kubeAPIServerConfig, apiExtensionsServer.GenericAPIServer, sharedInformers)
	if err != nil {
		return nil, err
	}

	kubeAPIServer.GenericAPIServer.PrepareRun()

	return kubeAPIServer.GenericAPIServer, nil
}

// CreateKubeAPIServer creates and wires a workable kube-apiserver
func CreateKubeAPIServer(kubeAPIServerConfig *master.Config, delegateAPIServer genericapiserver.DelegationTarget, sharedInformers informers.SharedInformerFactory) (*master.Master, error) {
	kubeAPIServer, err := kubeAPIServerConfig.Complete().New(delegateAPIServer)
	if err != nil {
		return nil, err
	}
	kubeAPIServer.GenericAPIServer.AddPostStartHook("start-kube-apiserver-informers", func(context genericapiserver.PostStartHookContext) error {
		sharedInformers.Start(context.StopCh)
		return nil
	})

	return kubeAPIServer, nil
}

func admissionPlugins() *admission.Plugins {
	plugins := &admission.Plugins{}

	server.RegisterAllAdmissionPlugins(plugins)
	admit.Register(plugins)
	alwayspullimages.Register(plugins)
	antiaffinity.Register(plugins)
	defaulttolerationseconds.Register(plugins)
	deny.Register(plugins)
	exec.Register(plugins)
	gc.Register(plugins)
	imagepolicy.Register(plugins)
	initialization.Register(plugins)
	initialresources.Register(plugins)
	limitranger.Register(plugins)
	autoprovision.Register(plugins)
	exists.Register(plugins)
	noderestriction.Register(plugins)
	label.Register(plugins)
	podnodeselector.Register(plugins)
	podpreset.Register(plugins)
	podtolerationrestriction.Register(plugins)
	resourcequota.Register(plugins)
	podsecuritypolicy.Register(plugins)
	scdeny.Register(plugins)
	serviceaccount.Register(plugins)
	setdefault.Register(plugins)
	webhook.Register(plugins)

	return plugins
}

// CreateKubeAPIServerConfig creates all the resources for running the API server, but runs none of them
func CreateKubeAPIServerConfig(etcdOptions genericoptions.EtcdOptions) (*master.Config, informers.SharedInformerFactory, error) {
	genericConfig, sharedInformers, err := BuildGenericConfig()
	if err != nil {
		return nil, nil, err
	}

	capabilities.Initialize(capabilities.Capabilities{
		AllowPrivileged: true,
		// TODO(vmarmol): Implement support for HostNetworkSources.
		PrivilegedSources: capabilities.PrivilegedSources{
			HostNetworkSources: []string{},
			HostPIDSources:     []string{},
			HostIPCSources:     []string{},
		},
		PerConnectionBandwidthLimitBytesPerSec: 0,
	})

	_, clusterIPRange, err := net.ParseCIDR("10.43.0.0/24")
	if err != nil {
		return nil, nil, err
	}

	serviceIPRange, apiServerServiceIP, err := master.DefaultServiceIPRange(*clusterIPRange)
	if err != nil {
		return nil, nil, err
	}

	storageFactory, err := BuildStorageFactory(etcdOptions)
	if err != nil {
		return nil, nil, err
	}

	etcdOptions.ApplyWithStorageFactoryTo(storageFactory, genericConfig)

	config := &master.Config{
		GenericConfig: genericConfig,

		APIResourceConfigSource: storageFactory.APIResourceConfigSource,
		StorageFactory:          storageFactory,
		EnableCoreControllers:   true,
		EventTTL:                1 * time.Hour,
		KubeletClientConfig: kubeletclient.KubeletClientConfig{
			Port:         ports.KubeletPort,
			ReadOnlyPort: ports.KubeletReadOnlyPort,
			PreferredAddressTypes: []string{
				// --override-hostname
				string(api.NodeHostName),

				// internal, preferring DNS if reported
				string(api.NodeInternalDNS),
				string(api.NodeInternalIP),

				// external, preferring DNS if reported
				string(api.NodeExternalDNS),
				string(api.NodeExternalIP),
			},
			EnableHttps: true,
			HTTPTimeout: time.Duration(5) * time.Second,
		},
		EnableUISupport:   true,
		EnableLogsSupport: true,

		ServiceIPRange:       serviceIPRange,
		APIServerServiceIP:   apiServerServiceIP,
		APIServerServicePort: 443,

		ServiceNodePortRange: utilnet.PortRange{Base: 30000, Size: 2768},

		MasterCount: 1,
	}

	return config, sharedInformers, nil
}

// BuildGenericConfig takes the master server options and produces the genericapiserver.Config associated with it
func BuildGenericConfig() (*genericapiserver.Config, informers.SharedInformerFactory, error) {
	genericConfig := genericapiserver.NewConfig(api.Codecs)
	genericConfig.LongRunningFunc = filters.BasicLongRunningRequestCheck(
		sets.NewString("watch", "proxy"),
		sets.NewString("attach", "exec", "proxy", "log", "portforward"),
	)

	kubeVersion := version.Get()
	genericConfig.Version = &kubeVersion

	//storageFactory, err := kubeapiserver.NewStorageFactory(storagebackend.Config{},
	//	"", api.Codecs,
	//	serverstorage.NewDefaultResourceEncodingConfig(api.Registry), nil,
	//	// FIXME: this GroupVersionResource override should be configurable
	//	[]schema.GroupVersionResource{batch.Resource("cronjobs").WithVersion("v2alpha1")},
	//	master.DefaultAPIResourceConfigSource(), nil)
	//if err != nil {
	//	return nil, nil, nil, fmt.Errorf("error in initializing storage factory: %s", err)
	//}

	// Use protobufs for self-communication.
	// Since not every generic apiserver has to support protobufs, we
	// cannot default to it in generic apiserver and need to explicitly
	// set it in kube-apiserver.
	genericConfig.LoopbackClientConfig = &rest.Config{}
	genericConfig.LoopbackClientConfig.ContentConfig.ContentType = "application/vnd.kubernetes.protobuf"

	client, err := internalclientset.NewForConfig(genericConfig.LoopbackClientConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create clientset: %v", err)
	}
	externalClient, err := clientset.NewForConfig(genericConfig.LoopbackClientConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create external clientset: %v", err)
	}
	sharedInformers := informers.NewSharedInformerFactory(client, 10*time.Minute)

	clientgoExternalClient, err := clientgoclientset.NewForConfig(genericConfig.LoopbackClientConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create real external clientset: %v", err)
	}
	//genericConfig.Authenticator, genericConfig.OpenAPIConfig.SecurityDefinitions, err = BuildAuthenticator(s, storageFactory, client, sharedInformers)
	//if err != nil {
	//return nil, nil, nil, fmt.Errorf("invalid authentication config: %v", err)
	//}

	//genericConfig.Authorizer, err = BuildAuthorizer(s, sharedInformers)
	//if err != nil {
	//return nil, nil, nil, fmt.Errorf("invalid authorization config: %v", err)
	//}

	pluginInitializer := kubeapiserveradmission.NewPluginInitializer(client,
		externalClient,
		sharedInformers,
		genericConfig.Authorizer,
		nil,
		api.Registry.RESTMapper(),
		quotainstall.NewRegistry(nil, nil))

	if err := initAdmissionControllers(clientgoExternalClient, genericConfig, pluginInitializer); err != nil {
		return nil, nil, fmt.Errorf("failed to initialize admission: %v", err)
	}

	return genericConfig, sharedInformers, nil
}

func initAdmissionControllers(externalClientSet clientgoclientset.Interface, serverCfg *server.Config, pluginInitializer admission.PluginInitializer) error {
	// TODO pass good stuff here
	pluginNames := []string{}
	configFileName := ""
	pluginsConfigProvider, err := admission.ReadAdmissionConfiguration(pluginNames, configFileName)
	if err != nil {
		return fmt.Errorf("failed to read plugin config: %v", err)
	}

	genericInitializer, err := initializer.New(externalClientSet, serverCfg.SharedInformerFactory, serverCfg.Authorizer)
	if err != nil {
		return err
	}
	admissionChain, err := admissionPlugins().NewFromPlugins(pluginNames,
		pluginsConfigProvider,
		admission.PluginInitializers{genericInitializer, pluginInitializer})
	if err != nil {
		return err
	}

	serverCfg.AdmissionControl = admissionChain
	return nil
}

// BuildAdmissionPluginInitializer constructs the admission plugin initializer
// BuildStorageFactory constructs the storage factory
func BuildStorageFactory(etcd genericoptions.EtcdOptions) (*serverstorage.DefaultStorageFactory, error) {
	storageFactory, err := kubeapiserver.NewStorageFactory(
		etcd.StorageConfig, etcd.DefaultStorageMediaType, api.Codecs,
		serverstorage.NewDefaultResourceEncodingConfig(api.Registry), nil,
		// FIXME: this GroupVersionResource override should be configurable
		[]schema.GroupVersionResource{batch.Resource("cronjobs").WithVersion("v2alpha1")},
		master.DefaultAPIResourceConfigSource(), flag.ConfigurationMap{
			"api/all": "true",
		})
	if err != nil {
		return nil, fmt.Errorf("error in initializing storage factory: %s", err)
	}

	return storageFactory, nil
}
