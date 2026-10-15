package storage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
)

type Record struct {
	Op    string `json:"op"`
	Key   string `json:"key"`
	Value []byte `json:"value,omitempty"`
}

type WAL struct {
	mu   sync.Mutex
	path string
	file *os.File
}

func OpenWAL(path string) (*WAL, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0600)
	if err != nil {
		return nil, fmt.Errorf("open wal: %w", err)
	}
	return &WAL{path: path, file: file}, nil
}

func (w *WAL) Append(record Record) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if _, err = w.file.Write(data); err != nil {
		return err
	}
	return w.file.Sync()
}

func (w *WAL) Replay(fn func(Record) error) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, err := w.file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	scanner := bufio.NewScanner(w.file)
	for scanner.Scan() {
		var record Record
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return fmt.Errorf("decode wal record: %w", err)
		}
		if err := fn(record); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	_, err := w.file.Seek(0, io.SeekEnd)
	return err
}

// Rewrite atomically replaces the log with one put record per key, so it matches restored state.
func (w *WAL) Rewrite(data map[string][]byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	tmpPath := w.path + ".tmp"
	tmp, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	writer := bufio.NewWriter(tmp)
	for key, value := range data {
		line, err := json.Marshal(Record{Op: "put", Key: key, Value: value})
		if err == nil {
			_, err = writer.Write(append(line, '\n'))
		}
		if err != nil {
			tmp.Close()
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// Windows cannot rename over an open file, so close the live log first.
	if err := w.file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, w.path); err != nil {
		return err
	}
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("reopen wal: %w", err)
	}
	w.file = file
	return nil
}

func (w *WAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Close()
}
