package mvcc

import (
	"fmt"
	"sort"
	"sync"
)

type Version struct {
	Timestamp uint64 `json:"timestamp"`
	Value     []byte `json:"value,omitempty"`
	Deleted   bool   `json:"deleted,omitempty"`
}

type Engine struct {
	mu     sync.RWMutex
	clock  uint64
	data   map[string][]Version
	active map[uint64]struct{}
}

type Transaction struct {
	engine *Engine
	Start  uint64
	writes map[string]*Version
	closed bool
}

func New() *Engine {
	return &Engine{data: make(map[string][]Version), active: make(map[uint64]struct{})}
}

func (e *Engine) Begin() *Transaction {
	e.mu.Lock()
	e.clock++
	start := e.clock
	e.active[start] = struct{}{}
	e.mu.Unlock()
	return &Transaction{engine: e, Start: start, writes: make(map[string]*Version)}
}

func (t *Transaction) Get(key string) ([]byte, bool) {
	if version, ok := t.writes[key]; ok {
		return append([]byte(nil), version.Value...), !version.Deleted
	}
	t.engine.mu.RLock()
	defer t.engine.mu.RUnlock()
	versions := t.engine.data[key]
	for i := len(versions) - 1; i >= 0; i-- {
		if versions[i].Timestamp <= t.Start {
			return append([]byte(nil), versions[i].Value...), !versions[i].Deleted
		}
	}
	return nil, false
}

func (t *Transaction) Put(key string, value []byte) {
	t.writes[key] = &Version{Value: append([]byte(nil), value...)}
}
func (t *Transaction) Delete(key string) { t.writes[key] = &Version{Deleted: true} }

func (t *Transaction) Commit() (uint64, error) {
	if t.closed {
		return 0, fmt.Errorf("transaction is closed")
	}
	e := t.engine
	e.mu.Lock()
	defer e.mu.Unlock()
	for key := range t.writes {
		versions := e.data[key]
		if len(versions) > 0 && versions[len(versions)-1].Timestamp > t.Start {
			t.closed = true
			delete(e.active, t.Start)
			return 0, fmt.Errorf("write conflict on key %q", key)
		}
	}
	e.clock++
	commit := e.clock
	keys := make([]string, 0, len(t.writes))
	for key := range t.writes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		version := *t.writes[key]
		version.Timestamp = commit
		e.data[key] = append(e.data[key], version)
	}
	t.closed = true
	delete(e.active, t.Start)
	return commit, nil
}

func (t *Transaction) Rollback() {
	if !t.closed {
		t.closed = true
		t.engine.mu.Lock()
		delete(t.engine.active, t.Start)
		t.engine.mu.Unlock()
	}
}

func (e *Engine) GC() {
	e.mu.Lock()
	defer e.mu.Unlock()
	oldest := e.clock + 1
	for timestamp := range e.active {
		if timestamp < oldest {
			oldest = timestamp
		}
	}
	for key, versions := range e.data {
		keep := 0
		for i := len(versions) - 1; i >= 0; i-- {
			if versions[i].Timestamp < oldest {
				keep = i
				break
			}
		}
		if keep > 0 {
			e.data[key] = append([]Version(nil), versions[keep:]...)
		}
	}
}
