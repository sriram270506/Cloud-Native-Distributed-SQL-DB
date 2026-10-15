package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Store struct {
	mu   sync.RWMutex
	data map[string][]byte
	wal  *WAL
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	wal, err := OpenWAL(path + ".wal")
	if err != nil {
		return nil, err
	}
	store := &Store{data: make(map[string][]byte), wal: wal}
	if err := wal.Replay(func(record Record) error {
		if record.Op == "delete" {
			delete(store.data, record.Key)
		} else {
			store.data[record.Key] = append([]byte(nil), record.Value...)
		}
		return nil
	}); err != nil {
		wal.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Put(key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.wal.Append(Record{Op: "put", Key: key, Value: value}); err != nil {
		return err
	}
	s.data[key] = append([]byte(nil), value...)
	return nil
}

func (s *Store) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.wal.Append(Record{Op: "delete", Key: key}); err != nil {
		return err
	}
	delete(s.data, key)
	return nil
}

func (s *Store) Get(key string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.data[key]
	return append([]byte(nil), value...), ok
}

func (s *Store) Scan(prefix string) map[string][]byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := make([]string, 0)
	for key := range s.data {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	result := make(map[string][]byte, len(keys))
	for _, key := range keys {
		result[key] = append([]byte(nil), s.data[key]...)
	}
	return result
}

func (s *Store) Keys(prefix string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := make([]string, 0)
	for key := range s.data {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func (s *Store) Snapshot() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return json.Marshal(s.data)
}

func (s *Store) Restore(data []byte) error {
	var values map[string][]byte
	if err := json.Unmarshal(data, &values); err != nil {
		return fmt.Errorf("decode snapshot: %w", err)
	}
	if values == nil {
		values = make(map[string][]byte)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.wal.Rewrite(values); err != nil {
		return fmt.Errorf("rewrite wal: %w", err)
	}
	s.data = values
	return nil
}

func (s *Store) Close() error { return s.wal.Close() }
