package sql

import (
	"testing"

	"github.com/sriram270506/cloud-native-distributed-sql-db/storage"
)

func newTestEngine(t *testing.T) *Engine {
	store, err := storage.Open(t.TempDir() + "/db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	db, _ := New(store)
	for _, statement := range []string{
		"CREATE TABLE users (id INTEGER, name TEXT)",
		"INSERT INTO users (id, name) VALUES (1, 'Ada')",
		"INSERT INTO users (id, name) VALUES (2, 'Grace')",
	} {
		if _, err := db.Execute(statement); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func TestBindRejectsInjection(t *testing.T) {
	db := newTestEngine(t)
	for _, attack := range []string{"x' OR '1'='1", "Ada' OR id = 2 OR name = 'x", "x order by name", "Ada'); DROP TABLE users; --"} {
		statement, err := Bind("SELECT * FROM users WHERE name = ?", []interface{}{attack})
		if err != nil {
			t.Fatal(err)
		}
		result, err := db.Execute(statement)
		if err != nil {
			t.Fatalf("%q: %v", attack, err)
		}
		if len(result.Rows) != 0 {
			t.Fatalf("injection %q matched rows: %+v", attack, result.Rows)
		}
	}
	if _, err := db.Execute("SELECT * FROM users"); err != nil {
		t.Fatal("table damaged: ", err)
	}
}

func TestBindStoresValuesVerbatim(t *testing.T) {
	db := newTestEngine(t)
	tricky := "O'Brien, Jr. and sons (or not)"
	statement, err := Bind("INSERT INTO users (id, name) VALUES (?, ?)", []interface{}{float64(3), tricky})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Execute(statement); err != nil {
		t.Fatal(err)
	}
	statement, _ = Bind("SELECT name FROM users WHERE name = ? AND id = ?", []interface{}{tricky, float64(3)})
	result, err := db.Execute(statement)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 || result.Rows[0]["name"] != tricky {
		t.Fatalf("unexpected result: %+v", result.Rows)
	}
}

func TestBindPlaceholderCount(t *testing.T) {
	if _, err := Bind("SELECT * FROM users WHERE id = ?", nil); err == nil {
		t.Fatal("missing parameter accepted")
	}
	if _, err := Bind("SELECT * FROM users WHERE name = '?'", []interface{}{"x"}); err == nil {
		t.Fatal("placeholder inside a literal was bound")
	}
}
