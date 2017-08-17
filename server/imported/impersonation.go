package imported

import (
	"net/http"

	"github.com/rancher/go-rancher/v3"
	"k8s.io/client-go/transport"
)

// TODO: set extras
// TODO: upstream a SetImpersonationHeaders function to k8s.io/client-go
func setImpersonationHeaders(req *http.Request, cluster *client.Cluster) {
	if len(req.Header.Get(transport.ImpersonateUserHeader)) != 0 {
		return
	}
	req.Header.Set(transport.ImpersonateUserHeader, cluster.Identity.Username)
	for _, group := range cluster.Identity.Groups {
		req.Header.Add(transport.ImpersonateGroupHeader, group)
	}
}
