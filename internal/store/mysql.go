package store

import (
	"database/sql"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/akl7777777/ip-intel/internal/model"

	_ "github.com/go-sql-driver/mysql"
)

type mysqlStore struct {
	db   *sql.DB
	ttl  time.Duration
	mu   sync.RWMutex
	stop chan struct{}
}

func NewMySQL(dsn string, ttl time.Duration) (Store, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS ip_cache (
			ip         VARCHAR(45) PRIMARY KEY,
			data       TEXT NOT NULL,
			source     VARCHAR(30) NOT NULL,
			updated_at BIGINT NOT NULL,
			INDEX idx_updated_at (updated_at)
		)
	`); err != nil {
		db.Close()
		return nil, err
	}

	s := &mysqlStore{
		db:   db,
		ttl:  ttl,
		stop: make(chan struct{}),
	}
	go s.cleanupLoop()

	log.Printf("[store] MySQL persistent cache opened (TTL: %s)", ttl)
	return s, nil
}

func (s *mysqlStore) Get(ip string) (*model.IPInfo, bool) {
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

func (s *mysqlStore) Set(ip string, info *model.IPInfo) {
	data, err := json.Marshal(info)
	if err != nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	_, _ = s.db.Exec(
		`INSERT INTO ip_cache (ip, data, source, updated_at) VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE data=VALUES(data), source=VALUES(source), updated_at=VALUES(updated_at)`,
		ip, string(data), info.Source, time.Now().Unix(),
	)
}

func (s *mysqlStore) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM ip_cache").Scan(&count); err != nil {
		return 0
	}
	return count
}

func (s *mysqlStore) Cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-s.ttl).Unix()
	result, err := s.db.Exec("DELETE FROM ip_cache WHERE updated_at <= ?", cutoff)
	if err != nil {
		log.Printf("[store] MySQL cleanup error: %v", err)
		return
	}
	if affected, _ := result.RowsAffected(); affected > 0 {
		log.Printf("[store] MySQL cleanup: removed %d expired entries", affected)
	}
}

func (s *mysqlStore) cleanupLoop() {
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

func (s *mysqlStore) Close() {
	close(s.stop)
	s.db.Close()
	log.Printf("[store] MySQL persistent cache closed")
}
