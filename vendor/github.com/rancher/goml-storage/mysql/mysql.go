package mysql

import (
	"context"
	"database/sql"

	"github.com/rancher/goml-storage/kv"
	"github.com/rancher/goml-storage/rdbms"
)

func init() {
	rdbms.Register("mysql", &mysql{})
}

type mysql struct {
}

func (m *mysql) Get(ctx context.Context, db *sql.DB, key string) (*kv.KeyValue, error) {
	value := kv.KeyValue{}
	row := db.QueryRowContext(ctx, where("name = ?"), key)
	err := scan(row.Scan, &value)
	if err == sql.ErrNoRows {
		return nil, nil
	}

	return &value, err
}

func (m *mysql) List(ctx context.Context, db *sql.DB, key string) ([]*kv.KeyValue, error) {
	rows, err := db.QueryContext(ctx, where("name like ?"), key+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	resp := []*kv.KeyValue{}
	for rows.Next() {
		value := kv.KeyValue{}
		if err := scan(rows.Scan, &value); err != nil {
			return nil, err
		}
		resp = append(resp, &value)
	}

	return resp, nil
}

func (m *mysql) Create(ctx context.Context, db *sql.DB, key string, value []byte, ttl uint64) error {
	_, err := db.ExecContext(ctx, "insert into key_value(name, value, revision) values(?, ?, 1)",
		key,
		[]byte(value))
	return err
}

func (m *mysql) Delete(ctx context.Context, db *sql.DB, key string, revision *int64) (*kv.KeyValue, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Commit()

	value, err := lock(ctx, tx, key)
	if err != nil {
		return nil, err
	}
	if value == nil || (revision != nil && value.Revision != *revision) {
		return nil, kv.ErrNotExists
	}

	_, err = tx.ExecContext(ctx, "delete from key_value where name = ?", key)
	return value, err
}

func (m *mysql) Update(ctx context.Context, db *sql.DB, key string, value []byte, revision int64) (*kv.KeyValue, *kv.KeyValue, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Commit()

	oldKv, err := lock(ctx, tx, key)
	if err != nil {
		return nil, nil, err
	}
	if oldKv == nil {
		return nil, nil, kv.ErrNotExists
	}

	if oldKv.Revision != revision {
		return nil, nil, rdbms.ErrRevisionMatch
	}

	_, err = tx.ExecContext(ctx, "update key_value set value = ?, revision = ? where name = ?", value, oldKv.Revision+1, key)
	if err != nil {
		return nil, nil, err
	}

	return oldKv, &kv.KeyValue{
		Key:      oldKv.Key,
		Value:    []byte(value),
		Revision: oldKv.Revision + 1,
	}, nil
}

func where(condition string) string {
	return "select name, value, revision from key_value where " + condition
}

func lock(ctx context.Context, tx *sql.Tx, key string) (*kv.KeyValue, error) {
	result := kv.KeyValue{}
	row := tx.QueryRowContext(ctx, where("name = ?")+" for update", key)
	err := scan(row.Scan, &result)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &result, err
}

type scanner func(dest ...interface{}) error

func scan(s scanner, out *kv.KeyValue) error {
	return s(&out.Key, &out.Value, &out.Revision)
}
