package authentication

import (
	"net/http"
	"k8s.io/apiserver/pkg/authentication/user"
	"github.com/rancher/netes/cluster"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/group"
	"fmt"
	"github.com/rancher/go-rancher/v3"
)

type Authenticator struct {
	clusterLookup *cluster.Lookup
}

func New(clusterLookup *cluster.Lookup) authenticator.Request {
	return group.NewAuthenticatedGroupAdder(&Authenticator{
		clusterLookup: clusterLookup,
	})
}

func (a *Authenticator) AuthenticateRequest(req *http.Request) (user.Info, bool, error) {
	cluster, ok := req.Context().Value("cluster").(*client.Cluster)
	if !ok {
		return nil, false, nil
	}

	attrs := map[string][]string{}
	for k, v := range cluster.Identity.Attributes {
		attrs[k] = []string{fmt.Sprint(v)}
	}

	return &user.DefaultInfo{
		Name: cluster.Identity.Username,
		UID: cluster.Identity.UserId,
		Groups: cluster.Identity.Groups,
		Extra: attrs,
	}, true, nil
}

