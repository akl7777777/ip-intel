package store

import (
	"fmt"
	"time"

	"github.com/akl7777777/ip-intel/internal/model"
)

// Store is the interface for persistent IP cache backends.
type Store interface {
	Get(ip string) (*model.IPInfo, bool)
	Set(ip string, info *model.IPInfo)
	Size() int
	Cleanup()
	Close()
}

// New creates a store based on the given type.
// Supported types: "sqlite", "mysql"
func New(storeType, dsn string, ttl time.Duration) (Store, error) {
	switch storeType {
	case "sqlite":
		return NewSQLite(dsn, ttl)
	case "mysql":
		return NewMySQL(dsn, ttl)
	default:
		return nil, fmt.Errorf("unsupported store type: %s (supported: sqlite, mysql)", storeType)
	}
}
