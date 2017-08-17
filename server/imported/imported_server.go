package imported

import (
	"context"
	"net/http"

	"github.com/rancher/go-rancher/v3"

	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http/httputil"
	"net/url"
	"strings"
)

type importedServer struct {
	cancel              context.CancelFunc
	cluster             *client.Cluster
	proxy               *httputil.ReverseProxy
	address             string
	url                 *url.URL
	tlsConfig           *tls.Config
	authorizationHeader string
}

func (s *importedServer) Close() {
}

func (s *importedServer) Handler() http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		for i := 1; i < 10; i++ {
			req.URL.Path = strings.TrimPrefix(req.URL.Path, fmt.Sprintf("/v3/clusters/1c%d", i))
		}

		req.Header.Set("Authorization", s.authorizationHeader)

		// TODO: return an error if there is no user for impersonation
		// if the user impersonation header is set with an empty string then authorization credentials are used
		cluster := req.Context().Value("cluster").(*client.Cluster)
		setImpersonationHeaders(req, cluster)

		s.proxyRequest(rw, req)
	})
}

func (s *importedServer) Cluster() *client.Cluster {
	return s.cluster
}

func New(cluster *client.Cluster) (*importedServer, error) {
	url, err := url.Parse(fmt.Sprintf("https://%s", cluster.K8sClientConfig.Address))
	if err != nil {
		return nil, err
	}

	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM([]byte(cluster.K8sClientConfig.CaCert))
	tlsConfig := &tls.Config{
		RootCAs: certPool,
	}

	proxy := httputil.NewSingleHostReverseProxy(url)
	proxy.Transport = &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	return &importedServer{
		cluster:             cluster,
		proxy:               proxy,
		address:             cluster.K8sClientConfig.Address,
		url:                 url,
		tlsConfig:           tlsConfig,
		authorizationHeader: fmt.Sprintf("Bearer %s", cluster.K8sClientConfig.BearerToken),
	}, nil
}
