package sql

import (
	"testing"

	"github.com/sriram270506/cloud-native-distributed-sql-db/storage"
)

func TestCRUD(t *testing.T) {
	store, err := storage.Open(t.TempDir() + "/db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	db, _ := New(store)
	for _, statement := range []string{"CREATE TABLE users (id INTEGER, name TEXT)", "INSERT INTO users (id, name) VALUES (1, 'Ada')", "UPDATE users SET name = 'Grace' WHERE id = 1"} {
		if _, err := db.Execute(statement); err != nil {
			t.Fatal(err)
		}
	}
	result, err := db.Execute("SELECT id, name FROM users WHERE id = 1")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 || result.Rows[0]["name"] != "Grace" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if _, err := db.Execute("DELETE FROM users WHERE id = 1"); err != nil {
		t.Fatal(err)
	}
}

func TestSelectFilteringGroupingAndOrdering(t *testing.T) {
	store, err := storage.Open(t.TempDir() + "/db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	db, _ := New(store)
	for _, statement := range []string{
		"CREATE TABLE products (id INTEGER, category TEXT, stock INTEGER, price TEXT)",
		"INSERT INTO products (id, category, stock, price) VALUES (1, 'food', 10, '2.50')",
		"INSERT INTO products (id, category, stock, price) VALUES (2, 'food', 20, '3.50')",
		"INSERT INTO products (id, category, stock, price) VALUES (3, 'home', 5, '10.00')",
	} {
		if _, err := db.Execute(statement); err != nil {
			t.Fatal(err)
		}
	}
	result, err := db.Execute("SELECT category, COUNT(*) AS products, SUM(stock) AS units FROM products WHERE stock >= 10 GROUP BY category ORDER BY units DESC")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 || result.Rows[0]["category"] != "food" || result.Rows[0]["units"] != "30" {
		t.Fatalf("unexpected grouped result: %+v", result)
	}
}

func TestSelectSupportsOrAndHaving(t *testing.T) {
	store, err := storage.Open(t.TempDir() + "/db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	db, _ := New(store)
	for _, statement := range []string{
		"CREATE TABLE products (id INTEGER, category TEXT, stock INTEGER)",
		"INSERT INTO products (id, category, stock) VALUES (1, 'food', 10)",
		"INSERT INTO products (id, category, stock) VALUES (2, 'food', 20)",
		"INSERT INTO products (id, category, stock) VALUES (3, 'home', 5)",
	} {
		if _, err := db.Execute(statement); err != nil {
			t.Fatal(err)
		}
	}
	result, err := db.Execute("SELECT category, COUNT(*) AS products FROM products WHERE category = 'food' OR stock = 5 GROUP BY category HAVING products >= 2")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 || result.Rows[0]["category"] != "food" || result.Rows[0]["products"] != "2" {
		t.Fatalf("unexpected OR/HAVING result: %+v", result)
	}
}

func TestSelectSupportsJoin(t *testing.T) {
	store, err := storage.Open(t.TempDir() + "/db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	db, _ := New(store)
	for _, statement := range []string{
		"CREATE TABLE customers (id INTEGER, name TEXT)",
		"CREATE TABLE orders (id INTEGER, customer_id INTEGER, total INTEGER)",
		"INSERT INTO customers (id, name) VALUES (1, 'Ada')",
		"INSERT INTO customers (id, name) VALUES (2, 'Grace')",
		"INSERT INTO orders (id, customer_id, total) VALUES (10, 1, 99)",
		"INSERT INTO orders (id, customer_id, total) VALUES (11, 2, 50)",
	} {
		if _, err := db.Execute(statement); err != nil {
			t.Fatal(err)
		}
	}
	result, err := db.Execute("SELECT customers.name, orders.total FROM customers JOIN orders ON customers.id = orders.customer_id WHERE orders.total > 75")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 || result.Rows[0]["customers.name"] != "Ada" || result.Rows[0]["orders.total"] != "99" {
		t.Fatalf("unexpected JOIN result: %+v", result)
	}
}
