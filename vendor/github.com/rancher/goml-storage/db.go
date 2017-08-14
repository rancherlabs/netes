package db

import (
	"context"
	"database/sql"

	"github.com/pkg/errors"
	"github.com/rancher/goml-storage/kv"
	"github.com/rancher/goml-storage/rdbms"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/apiserver/pkg/storage/storagebackend"
	"k8s.io/apiserver/pkg/storage/storagebackend/factory"
	"k8s.io/apiserver/pkg/storage/value"
)

var ErrNoDSN = errors.New("DB DSN must be set as ServerList")

func NewRDBMSStorage(c storagebackend.Config) (storage.Interface, factory.DestroyFunc, error) {
	if len(c.ServerList) != 2 {
		return nil, nil, ErrNoDSN
	}

	driverName, dsn := c.ServerList[0], c.ServerList[1]

	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "Failed to create DB(%s) connection", driverName)
	}

	ctx, cancel := context.WithCancel(context.Background())
	destroyFunc := func() {
		cancel()
		db.Close()
	}

	transformer := c.Transformer
	if transformer == nil {
		transformer = value.NewMutableTransformer(value.IdentityTransformer)
	}

	dbClient, err := rdbms.NewClient(ctx, driverName, db)
	if err != nil {
		return nil, nil, err
	}

	return kv.New(dbClient, c.Codec, c.Prefix, transformer), destroyFunc, nil
}
