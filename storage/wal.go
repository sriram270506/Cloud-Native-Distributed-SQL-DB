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
	file *os.File
}

func OpenWAL(path string) (*WAL, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0600)
	if err != nil {
		return nil, fmt.Errorf("open wal: %w", err)
	}
	return &WAL{file: file}, nil
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

func (w *WAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Close()
}
