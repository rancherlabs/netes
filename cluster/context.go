package cluster

import (
	"context"

	"github.com/rancher/go-rancher/v3"
)

func GetCluster(ctx context.Context) *client.Cluster {
	cluster, _ := ctx.Value("cluster").(*client.Cluster)
	return cluster
}

func StoreCluster(ctx context.Context, cluster *client.Cluster) context.Context {
	return context.WithValue(ctx, "cluster", cluster)
}
