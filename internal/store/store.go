package store

import (
	"database/sql"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/akl7777777/ip-intel/internal/model"

	_ "modernc.org/sqlite"
)

// Store is a persistent SQLite cache for IP lookup results.
type Store struct {
	db  *sql.DB
	ttl time.Duration
	mu  sync.RWMutex
	stop chan struct{}
}

// New creates a new persistent store at the given path.
func New(dbPath string, ttl time.Duration) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	// SQLite optimizations
	db.SetMaxOpenConns(1) // SQLite doesn't handle concurrent writes well
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec("PRAGMA synchronous=NORMAL"); err != nil {
		db.Close()
		return nil, err
	}

	// Create table
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS ip_cache (
			ip         TEXT PRIMARY KEY,
			data       TEXT NOT NULL,
			source     TEXT NOT NULL,
			updated_at INTEGER NOT NULL
		)
	`); err != nil {
		db.Close()
		return nil, err
	}

	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_updated_at ON ip_cache(updated_at)`); err != nil {
		db.Close()
		return nil, err
	}

	s := &Store{
		db:   db,
		ttl:  ttl,
		stop: make(chan struct{}),
	}

	// Background cleanup every hour
	go s.cleanupLoop()

	log.Printf("[store] Persistent cache opened: %s (TTL: %s)", dbPath, ttl)
	return s, nil
}

// Get retrieves an IP lookup result from the persistent cache.
// Returns nil, false if not found or expired.
func (s *Store) Get(ip string) (*model.IPInfo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cutoff := time.Now().Add(-s.ttl).Unix()
	var data string
	err := s.db.QueryRow(
		"SELECT data FROM ip_cache WHERE ip = ? AND updated_at > ?",
		ip, cutoff,
	).Scan(&data)
	if err != nil {
		return nil, false
	}

	var info model.IPInfo
	if json.Unmarshal([]byte(data), &info) != nil {
		return nil, false
	}
	return &info, true
}

// Set stores an IP lookup result in the persistent cache.
func (s *Store) Set(ip string, info *model.IPInfo) {
	data, err := json.Marshal(info)
	if err != nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	_, _ = s.db.Exec(
		`INSERT INTO ip_cache (ip, data, source, updated_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(ip) DO UPDATE SET data=excluded.data, source=excluded.source, updated_at=excluded.updated_at`,
		ip, string(data), info.Source, time.Now().Unix(),
	)
}

// Size returns the number of entries in the persistent cache.
func (s *Store) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM ip_cache").Scan(&count); err != nil {
		return 0
	}
	return count
}

// Cleanup removes expired entries.
func (s *Store) Cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-s.ttl).Unix()
	result, err := s.db.Exec("DELETE FROM ip_cache WHERE updated_at <= ?", cutoff)
	if err != nil {
		log.Printf("[store] Cleanup error: %v", err)
		return
	}
	if affected, _ := result.RowsAffected(); affected > 0 {
		log.Printf("[store] Cleanup: removed %d expired entries", affected)
	}
}

func (s *Store) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.Cleanup()
		case <-s.stop:
			return
		}
	}
}

// Close closes the database connection and stops background cleanup.
func (s *Store) Close() {
	close(s.stop)
	s.db.Close()
	log.Printf("[store] Persistent cache closed")
}
