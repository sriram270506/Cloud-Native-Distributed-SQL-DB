package storage

import (
	"path/filepath"
	"testing"
)

func TestRestoreIsDurable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put("keep", []byte("1")); err != nil {
		t.Fatal(err)
	}
	before, _ := store.Snapshot()
	if err := store.Put("rolled-back", []byte("2")); err != nil {
		t.Fatal(err)
	}
	if err := store.Restore(before); err != nil {
		t.Fatal(err)
	}
	if err := store.Put("after", []byte("3")); err != nil {
		t.Fatal(err)
	}
	store.Close()

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, ok := reopened.Get("rolled-back"); ok {
		t.Fatal("rolled-back write survived restart")
	}
	for _, key := range []string{"keep", "after"} {
		if _, ok := reopened.Get(key); !ok {
			t.Fatalf("%s missing after restart", key)
		}
	}
}
