package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"reflect"
	"testing"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/ports"
)

// A recording driver exercises database/sql's real transaction lifecycle.
type recordingConnection struct {
	events    []string
	active    bool
	beginErr  error
	commitErr error
}

type recordingConnector struct{ conn *recordingConnection }

func (c recordingConnector) Connect(context.Context) (driver.Conn, error) { return c.conn, nil }
func (c recordingConnector) Driver() driver.Driver                        { return recordingDriver{c.conn} }

type recordingDriver struct{ conn *recordingConnection }

func (d recordingDriver) Open(string) (driver.Conn, error) { return d.conn, nil }
func (c *recordingConnection) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (c *recordingConnection) Close() error { return nil }
func (c *recordingConnection) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}
func (c *recordingConnection) BeginTx(_ context.Context, opts driver.TxOptions) (driver.Tx, error) {
	c.events = append(c.events, "begin")
	if opts.Isolation != driver.IsolationLevel(sql.LevelSerializable) {
		return nil, errors.New("expected serializable transaction")
	}
	if c.beginErr != nil {
		return nil, c.beginErr
	}
	c.active = true
	return c, nil
}
func (c *recordingConnection) Commit() error {
	c.events = append(c.events, "commit")
	c.active = false
	return c.commitErr
}
func (c *recordingConnection) Rollback() error {
	c.events = append(c.events, "rollback")
	c.active = false
	return nil
}
func (c *recordingConnection) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	if !c.active {
		return nil, errors.New("write escaped the transaction")
	}
	c.events = append(c.events, "write")
	return driver.RowsAffected(1), nil
}

func TestUnitOfWorkLifecycle(t *testing.T) {
	failure := errors.New("injected failure")
	for _, tc := range []struct {
		name                         string
		beginErr, workErr, commitErr error
		want                         []string
	}{
		{"commit", nil, nil, nil, []string{"begin", "write", "commit"}},
		{"rollback", nil, failure, nil, []string{"begin", "write", "rollback"}},
		{"begin failure", failure, nil, nil, []string{"begin"}},
		{"commit failure", nil, nil, failure, []string{"begin", "write", "commit"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn := &recordingConnection{beginErr: tc.beginErr, commitErr: tc.commitErr}
			database := sql.OpenDB(recordingConnector{conn})
			defer database.Close()
			err := New(database).WithinTx(context.Background(), func(repos ports.Repositories) error {
				if err := repos.Users().DeleteUser(context.Background(), 7); err != nil {
					return err
				}
				return tc.workErr
			})
			if tc.beginErr != nil || tc.workErr != nil || tc.commitErr != nil {
				if !errors.Is(err, failure) {
					t.Fatalf("error = %v, want injected error", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(conn.events, tc.want) {
				t.Fatalf("events = %v, want %v", conn.events, tc.want)
			}
		})
	}
}

func TestUnitOfWorkRollsBackPanic(t *testing.T) {
	conn := &recordingConnection{}
	database := sql.OpenDB(recordingConnector{conn})
	defer database.Close()
	defer func() {
		if recover() != "boom" {
			t.Error("expected original panic to propagate")
		}
		if !reflect.DeepEqual(conn.events, []string{"begin", "rollback"}) {
			t.Fatalf("events = %v", conn.events)
		}
	}()
	_ = New(database).WithinTx(context.Background(), func(ports.Repositories) error { panic("boom") })
}
