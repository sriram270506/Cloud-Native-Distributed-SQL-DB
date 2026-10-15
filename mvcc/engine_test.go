package mvcc

import "testing"

func TestSnapshotIsolationAndConflict(t *testing.T) {
	e := New()
	seed := e.Begin()
	seed.Put("x", []byte("one"))
	if _, err := seed.Commit(); err != nil {
		t.Fatal(err)
	}
	reader := e.Begin()
	writer := e.Begin()
	writer.Put("x", []byte("two"))
	if _, err := writer.Commit(); err != nil {
		t.Fatal(err)
	}
	value, ok := reader.Get("x")
	if !ok || string(value) != "one" {
		t.Fatalf("snapshot read = %q, %v", value, ok)
	}
	conflict := e.Begin()
	conflict.Put("x", []byte("three"))
	stale := e.Begin()
	stale.Put("x", []byte("four"))
	if _, err := conflict.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := stale.Commit(); err == nil {
		t.Fatal("expected write conflict")
	}
	reader.Rollback()
	stale.Rollback()
}
