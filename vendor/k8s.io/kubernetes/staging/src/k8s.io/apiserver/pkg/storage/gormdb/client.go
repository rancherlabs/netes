package gormdb

import (
	"database/sql"
	"sync"

	"golang.org/x/net/context"
	"k8s.io/apiserver/pkg/storage/kv"
	"fmt"
)

const chanSize = 1000

type watchChan chan kv.WatchResponse
type scanner func(dest ...interface{}) error

func NewClient(ctx context.Context, db *sql.DB) kv.Client {
	client := &client{
		db:       db,
		events:   make(chan kv.Event, chanSize),
		watchers: map[string][]watchChan{},
	}
	go client.watchEvents(ctx)
	return client
}

type client struct {
	sync.Mutex
	db       *sql.DB
	events   chan kv.Event
	watchers map[string][]watchChan
}

func (c *client) Get(ctx context.Context, key string) (*kv.KeyValue, error) {
	value := kv.KeyValue{}
	row := c.db.QueryRowContext(ctx, where("name = ?"), key)
	err := scan(row.Scan, &value)
	if err == sql.ErrNoRows {
		return nil, nil
	}

	return &value, err
}

func scan(s scanner, out *kv.KeyValue) error {
	return s(&out.Key, &out.Value, &out.Revision)
}

func where(condition string) string {
	return "select name, value, revision from key_value where " + condition
}

func (c *client) List(ctx context.Context, key string) (*kv.ListResponse, error) {
	resp := &kv.ListResponse{}

	rows, err := c.db.QueryContext(ctx, where("name like ?"), key+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		value := kv.KeyValue{}
		if err := scan(rows.Scan, &value); err != nil {
			return nil, err
		}
		resp.Kvs = append(resp.Kvs, &value)
	}

	return resp, nil
}

func (c *client) Create(ctx context.Context, key, value string, ttl uint64) (*kv.KeyValue, error) {
	_, err := c.db.ExecContext(ctx, "insert into key_value(name, value, revision) values(?, ?, 1)",
		key,
		[]byte(value))
	// TODO: Check for specific error? Don't just assume the key is taken
	if err != nil {
		fmt.Println("Create bad", err)
		return nil, kv.ErrExists
	}

	result := &kv.KeyValue{
		Key:      key,
		Value:    []byte(value),
		Revision: 1,
	}
	c.created(result)
	return result, nil
}

func (c *client) Delete(ctx context.Context, key string) (*kv.KeyValue, error) {
	return c.deleteVersion(ctx, key, nil)
}

func (c *client) lock(ctx context.Context, tx *sql.Tx, key string) (*kv.KeyValue, error) {
	result := kv.KeyValue{}
	row := tx.QueryRowContext(ctx, where("name = ?")+" for update", key)
	err := scan(row.Scan, &result)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &result, err
}

func (c *client) DeleteVersion(ctx context.Context, key string, revision int64) error {
	_, err := c.deleteVersion(ctx, key, &revision)
	return err
}

func (c *client) deleteVersion(ctx context.Context, key string, revision *int64) (*kv.KeyValue, error) {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Commit()

	value, err := c.lock(ctx, tx, key)
	if err != nil {
		return nil, err
	}
	if value == nil || (revision != nil && value.Revision != *revision) {
		return nil, kv.ErrNotExists
	}

	_, err = tx.ExecContext(ctx, "delete from key_value where name = ?", key)
	if err != nil {
		return nil, err
	}

	c.deleted(value)
	return value, nil
}

func (c *client) Update(ctx context.Context, key, value string, revision int64, ttl uint64) (*kv.KeyValue, error) {
	committed := false
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if !committed {
			tx.Commit()
		}
	}()

	oldKv, err := c.lock(ctx, tx, key)
	if err != nil {
		return nil, err
	}
	if oldKv == nil {
		committed = true
		if err := tx.Rollback(); err != nil {
			return nil, err
		}
		return c.Create(ctx, key, value, ttl)
	}

	if oldKv.Revision != revision {
		return nil, kv.ErrNotExists
	}

	_, err = tx.ExecContext(ctx, "update key_value set value = ?, revision = ? where name = ?", value, oldKv.Revision+1, key)
	if err != nil {
		return nil, err
	}

	result := &kv.KeyValue{
		Key:      oldKv.Key,
		Value:    []byte(value),
		Revision: oldKv.Revision + 1,
	}
	c.updated(oldKv, result)
	return result, nil
}
