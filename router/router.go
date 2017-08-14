package router

import (
	"encoding/json"
	"net/http"

	"github.com/rancher/go-rancher/v3"
	"github.com/rancher/netes/cluster"
	"github.com/rancher/netes/server"
	"github.com/rancher/netes/types"
	"fmt"
	"context"
)

type Router struct {
	clusterLookup *cluster.Lookup
	serverFactory *server.Factory
}

func New(config *types.GlobalConfig) *Router {
	return &Router{
		clusterLookup: config.Lookup,
		serverFactory: server.NewFactory(config),
	}
}

func (r *Router) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	fmt.Println("!!!!", req.Method, req.URL.String())
	cluster, err := r.clusterLookup.Lookup(req)
	if err != nil {
		response(rw, http.StatusInternalServerError, err.Error())
		return
	}

	if cluster == nil {
		response(rw, http.StatusNotFound, "No cluster available")
		return
	}


	handler, err := r.serverFactory.Get(cluster)
	if err != nil {
		response(rw, http.StatusInternalServerError, err.Error())
		return
	}

	ctx := context.WithValue(req.Context(), "cluster", cluster)
	handler.ServeHTTP(rw, req.WithContext(ctx))
}

func response(rw http.ResponseWriter, code int, message string) {
	rw.WriteHeader(code)
	rw.Header().Set("content-type", "application/json")
	json.NewEncoder(rw).Encode(&client.Error{
		Status:  int64(code),
		Message: message,
	})
}
