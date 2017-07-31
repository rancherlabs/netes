package factory

import (
	"context"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/apiserver/pkg/storage/gormdb"
	"k8s.io/apiserver/pkg/storage/kv"
	"k8s.io/apiserver/pkg/storage/storagebackend"
	"k8s.io/apiserver/pkg/storage/value"
)

type KeyValueRecord struct {
	Name      string `gorm:"name;unique_index"`
	Value    []byte `gorm:"value"`
	Revision int64  `gorm:"revision"`
}

func (KeyValueRecord) TableName() string {
	return "key_value"
}

func newRDBMSStorage(c storagebackend.Config) (storage.Interface, DestroyFunc, error) {
	ctx, cancel := context.WithCancel(context.Background())
	db, err := gorm.Open("mysql", "cattle:cattle@tcp(localhost:3306)/cattle")
	if err != nil {
		return nil, nil, err
	}

	// TODO get rid of gorm
	db.AutoMigrate(&KeyValueRecord{})

	kvClient := gormdb.NewClient(ctx, db.DB())

	destroyFunc := func() {
		cancel()
		db.Close()
	}

	transformer := c.Transformer
	if transformer == nil {
		transformer = value.NewMutableTransformer(value.IdentityTransformer)
	}

	return kv.New(kvClient, c.Codec, c.Prefix, transformer), destroyFunc, nil
}
