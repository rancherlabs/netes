package server

import (
	"net/http"

	"github.com/docker/docker/pkg/locker"
	"github.com/rancher/go-rancher/v3"
	"github.com/rancher/netes/server/embedded"
	"github.com/rancher/netes/types"
	"golang.org/x/sync/syncmap"
)

type Factory struct {
	servers    syncmap.Map
	serverLock *locker.Locker
	config     *types.GlobalConfig
}

func NewFactory(config *types.GlobalConfig) *Factory {
	return &Factory{
		serverLock: locker.New(),
		config:     config,
	}
}

func (s *Factory) Get(cluster *client.Cluster) (http.Handler, error) {
	server, ok := s.servers.Load(cluster.Id)
	if ok {
		return server.(Server).Handler(), nil
	}

	s.serverLock.Lock("cluster." + cluster.Id)
	defer s.serverLock.Unlock("cluster." + cluster.Id)

	server, err := s.newServer(cluster)
	if err != nil {
		return nil, err
	}

	server, _ = s.servers.LoadOrStore(cluster.Id, server)
	return server.(Server).Handler(), nil
}

func (s *Factory) newServer(c *client.Cluster) (Server, error) {
	if c.Embedded {
		return embedded.New(s.config, c, s.config.Lookup)
	}

	panic("psst: I don't know what I'm doing. Don't tell anyone.")
}
