// Package store encapsulates all database interaction.
package store

import (
	"bytes"
	"database/sql/driver"
	"encoding/csv"
	"fmt"
	"io"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	"github.com/pkg/errors"
	"github.com/smartcontractkit/external-initiator/store/migrations"
)

const sqlDialect = "postgres"

// SQLStringArray is a string array stored in the database as comma separated values.
type SQLStringArray []string

// Scan implements the sql Scanner interface.
func (arr *SQLStringArray) Scan(src interface{}) error {
	if src == nil {
		*arr = nil
	}
	v, err := driver.String.ConvertValue(src)
	if err != nil {
		return errors.New("failed to scan StringArray")
	}
	str, ok := v.(string)
	if !ok {
		return nil
	}

	buf := bytes.NewBufferString(str)
	r := csv.NewReader(buf)
	ret, err := r.Read()
	if err != nil && err != io.EOF {
		return errors.Wrap(err, "badly formatted csv string array")
	}
	*arr = ret
	return nil
}

// Value implements the driver Valuer interface.
func (arr SQLStringArray) Value() (driver.Value, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	err := w.Write(arr)
	if err != nil && err != io.EOF {
		return nil, errors.Wrap(err, "csv encoding of string array")
	}
	w.Flush()
	return buf.String(), nil
}

// SQLBytes is a byte slice stored in the database as a string.
type SQLBytes []byte

// Scan implements the sql Scanner interface.
func (bytes *SQLBytes) Scan(src interface{}) error {
	if src == nil {
		*bytes = nil
	}

	str, ok := src.(string)
	if !ok {
		return errors.New("failed to scan string")
	}

	*bytes = []byte(str)
	return nil
}

// Value implements the driver Valuer interface.
func (bytes SQLBytes) Value() (driver.Value, error) {
	return string(bytes), nil
}

// Client holds a connection to the database.
type Client struct {
	db *gorm.DB
}

// ConnectToDB attempts to connect to the database URI provided,
// and returns a new Client instance if successful.
func ConnectToDb(uri string) (*Client, error) {
	db, err := gorm.Open(sqlDialect, uri)
	if err != nil {
		return nil, fmt.Errorf("unable to open %s for gorm DB: %+v", uri, err)
	}
	if err = migrations.Migrate(db); err != nil {
		return nil, errors.Wrap(err, "newDBStore#Migrate")
	}
	store := &Client{
		db: db.Set("gorm:auto_preload", true),
	}
	return store, nil
}

// Close will close the connection to the database.
func (client Client) Close() error {
	return client.db.Close()
}

// DB return the underlying gorm db
func (client Client) DB() *gorm.DB {
	return client.db
}
