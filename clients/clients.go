package clients

import (
	"fmt"
	"time"

	"github.com/rancher/go-rancher/v3"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"
	internalinformers "k8s.io/kubernetes/pkg/client/informers/informers_generated/internalversion"
)

type ClientSetSet struct {
	LoopbackClientConfig    rest.Config
	InternalClient          internalclientset.Interface
	InternalSharedInformers internalinformers.SharedInformerFactory
	ExternalClient          clientset.Interface
	ExternalSharedInformers informers.SharedInformerFactory
}

func New(cluster *client.Cluster) (*ClientSetSet, error) {
	var err error

	c := &ClientSetSet{
		LoopbackClientConfig: rest.Config{
			Host: "http://localhost:8089/v3/clusters/1c1/",
			//Prefix: "/v3/clusters/1c1/",
			ContentConfig: rest.ContentConfig{
				ContentType: "application/vnd.kubernetes.protobuf",
			},
		},
	}

	c.InternalClient, err = internalclientset.NewForConfig(&c.LoopbackClientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %v", err)
	}
	c.ExternalClient, err = clientset.NewForConfig(&c.LoopbackClientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create external clientset: %v", err)
	}

	c.InternalSharedInformers = internalinformers.NewSharedInformerFactory(c.InternalClient, 10*time.Minute)
	c.ExternalSharedInformers = informers.NewSharedInformerFactory(c.ExternalClient, 10*time.Minute)

	return c, err

}
