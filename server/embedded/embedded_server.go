package embedded

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/pkg/errors"
	"github.com/rancher/go-rancher/v3"
	"github.com/rancher/netes/authentication"
	"github.com/rancher/netes/authorization"
	"github.com/rancher/netes/clients"
	"github.com/rancher/netes/cluster"
	"github.com/rancher/netes/server/admission"
	"github.com/rancher/netes/store"
	"github.com/rancher/netes/types"
	utilnet "k8s.io/apimachinery/pkg/util/net"
	"k8s.io/apimachinery/pkg/util/sets"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/apiserver/pkg/server/filters"
	"k8s.io/apiserver/pkg/server/storage"
	"k8s.io/kubernetes/pkg/api"
	kubeletclient "k8s.io/kubernetes/pkg/kubelet/client"
	"k8s.io/kubernetes/pkg/master"
	"k8s.io/kubernetes/pkg/master/ports"

	// Enable registering packages
	"strings"

	_ "github.com/rancher/goml-storage/mysql"
)

type embeddedServer struct {
	master  *master.Master
	cluster *client.Cluster
	cancel  context.CancelFunc
}

func (e *embeddedServer) Close() {
	e.cancel()
}

func (e *embeddedServer) Handler() http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		c := cluster.GetCluster(req.Context())
		req.URL.Path = strings.TrimPrefix(req.URL.Path, "/k8s/clusters/" + c.Id)
		e.master.GenericAPIServer.Handler.ServeHTTP(rw, req)
	})
}

func (e *embeddedServer) Cluster() *client.Cluster {
	return e.cluster
}

func New(config *types.GlobalConfig, cluster *client.Cluster, lookup *cluster.Lookup) (*embeddedServer, error) {
	storageFactory, err := store.StorageFactory(
		fmt.Sprintf("/k8s/cluster/%s", cluster.Uuid),
		config)
	if err != nil {
		return nil, err
	}

	clientsetset, err := clients.New(cluster)
	if err != nil {
		return nil, err
	}

	genericApiServerConfig, err := genericConfig(config, cluster, lookup, storageFactory, clientsetset)
	if err != nil {
		return nil, err
	}

	serviceIPRange, apiServerServiceIP, err := serviceNet(config, cluster)
	if err != nil {
		return nil, errors.Wrap(err, "Invalid service net cidr")
	}

	masterConfig := &master.Config{
		GenericConfig: genericApiServerConfig,

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

	kubeAPIServer, err := masterConfig.Complete().New(genericapiserver.EmptyDelegate, nil)
	kubeAPIServer.GenericAPIServer.AddPostStartHook("start-kube-apiserver-informers", func(context genericapiserver.PostStartHookContext) error {
		clientsetset.Start(context.StopCh)
		return nil
	})
	kubeAPIServer.GenericAPIServer.PrepareRun()

	ctx, cancel := context.WithCancel(context.Background())

	kubeAPIServer.GenericAPIServer.RunPostStartHooks(ctx.Done())
	//go controllermanager.Start(clientsetset, ctx.Done())

	return &embeddedServer{
		master:  kubeAPIServer,
		cluster: cluster,
		cancel:  cancel,
	}, nil
}

func serviceNet(config *types.GlobalConfig, cluster *client.Cluster) (net.IPNet, net.IP, error) {
	cidr := types.FirstNotEmpty(cluster.K8sServerConfig.ServiceNetCidr, config.ServiceNetCidr)
	_, cidrNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return net.IPNet{}, nil, err
	}

	return master.DefaultServiceIPRange(*cidrNet)
}

func genericConfig(config *types.GlobalConfig, cluster *client.Cluster, lookup *cluster.Lookup,
	storageFactory storage.StorageFactory, clientsetset *clients.ClientSetSet) (*genericapiserver.Config, error) {
	authz, err := authorization.New()
	if err != nil {
		return nil, err
	}

	admissions, err := admission.New(config, cluster, authz, clientsetset)
	if err != nil {
		return nil, err
	}

	genericApiServerConfig := genericapiserver.NewConfig(api.Codecs)
	genericApiServerConfig.LongRunningFunc = filters.BasicLongRunningRequestCheck(
		sets.NewString("watch", "proxy"),
		sets.NewString("attach", "exec", "proxy", "log", "portforward"),
	)
	genericApiServerConfig.LoopbackClientConfig = &clientsetset.LoopbackClientConfig
	genericApiServerConfig.AdmissionControl = admissions
	genericApiServerConfig.Authorizer = authz
	genericApiServerConfig.RESTOptionsGetter = &store.RESTOptionsFactory{storageFactory}
	genericApiServerConfig.Authenticator = authentication.New(lookup)
	genericApiServerConfig.Authorizer = authz
	genericApiServerConfig.PublicAddress = net.ParseIP("169.254.169.250")
	genericApiServerConfig.ReadWritePort = 81
	if err != nil {
		return nil, err
	}

	return genericApiServerConfig, nil
}
