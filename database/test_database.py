"""Test cases for the e-commerce schema: constraints, cascades and query results.

Run from the repository root:  python -m unittest discover -s database -v
"""
import sqlite3
import unittest
from pathlib import Path

from run_queries import build, parse_queries

HERE = Path(__file__).resolve().parent


class SchemaTest(unittest.TestCase):
    def setUp(self):
        self.db = sqlite3.connect(":memory:")
        build(self.db)

    def tearDown(self):
        self.db.close()

    def assertRejected(self, sql, params=()):
        with self.assertRaises(sqlite3.IntegrityError):
            self.db.execute(sql, params)

    def count(self, sql, params=()):
        return self.db.execute(sql, params).fetchone()[0]

    def query(self, qid):
        for found, _, _, sql in parse_queries((HERE / "queries.sql").read_text()):
            if found == qid:
                return self.db.execute(sql).fetchall()
        raise KeyError(qid)

    # --- population ---------------------------------------------------------
    def test_seed_row_counts(self):
        expected = {"customers": 10, "addresses": 10, "products": 10, "orders": 10, "order_items": 12, "payments": 10}
        for table, rows in expected.items():
            self.assertEqual(self.count(f"SELECT COUNT(*) FROM {table}"), rows, table)

    def test_no_dangling_foreign_keys(self):
        self.assertEqual(self.db.execute("PRAGMA foreign_key_check").fetchall(), [])

    # --- PRIMARY KEY / UNIQUE -----------------------------------------------
    def test_duplicate_primary_key_rejected(self):
        self.assertRejected("INSERT INTO products VALUES (1, 'Dup', 'tools', 1, 1, '2024-01-01', '2024-01-01')")

    def test_duplicate_email_rejected(self):
        self.assertRejected("INSERT INTO customers VALUES (99, 'A', 'B', 'emma.johnson@example.com', '2024-01-01', '2024-01-01', 'active')")

    def test_same_product_twice_on_one_order_rejected(self):
        self.assertRejected("INSERT INTO order_items VALUES (99, 1, 1, 1, 19.99, 19.99, '2024-06-01')")

    # --- FOREIGN KEY ---------------------------------------------------------
    def test_order_for_unknown_customer_rejected(self):
        self.assertRejected("INSERT INTO orders VALUES (99, 999, '2024-06-01', 0, 'pending', '2024-06-01', '2024-06-01')")

    def test_product_on_an_order_cannot_be_deleted(self):
        self.assertRejected("DELETE FROM products WHERE product_id = 1")

    # --- CHECK ---------------------------------------------------------------
    def test_check_constraints(self):
        cases = {
            "price must be positive": "INSERT INTO products VALUES (99, 'Free', 'tools', 0, 1, '2024-01-01', '2024-01-01')",
            "stock cannot be negative": "INSERT INTO products VALUES (99, 'Neg', 'tools', 5, -1, '2024-01-01', '2024-01-01')",
            "email format": "INSERT INTO customers VALUES (99, 'A', 'B', 'not-an-email', '2024-01-01', '2024-01-01', 'active')",
            "status list": "INSERT INTO customers VALUES (99, 'A', 'B', 'a@b.co', '2024-01-01', '2024-01-01', 'deleted')",
            "updated after created": "INSERT INTO customers VALUES (99, 'A', 'B', 'a@b.co', '2024-02-01', '2024-01-01', 'active')",
            "quantity positive": "INSERT INTO order_items VALUES (99, 2, 1, 0, 19.99, 0, '2024-06-02')",
            "line total = qty x price": "INSERT INTO order_items VALUES (99, 2, 1, 2, 19.99, 10, '2024-06-02')",
            "payment method list": "INSERT INTO payments VALUES (99, 1, 'bitcoin', 5, '2024-06-01', 'paid', 'tx9999')",
        }
        for name, sql in cases.items():
            with self.subTest(name):
                self.assertRejected(sql)

    def test_valid_row_accepted(self):
        self.db.execute("INSERT INTO order_items VALUES (99, 2, 1, 2, 19.99, 39.98, '2024-06-02')")
        self.assertEqual(self.count("SELECT COUNT(*) FROM order_items WHERE order_id = 2"), 2)

    # --- ON DELETE CASCADE ---------------------------------------------------
    def test_deleting_customer_cascades(self):
        self.db.execute("DELETE FROM customers WHERE customer_id = 4")
        self.assertEqual(self.count("SELECT COUNT(*) FROM orders WHERE customer_id = 4"), 0)
        self.assertEqual(self.count("SELECT COUNT(*) FROM order_items WHERE order_id = 4"), 0)
        self.assertEqual(self.count("SELECT COUNT(*) FROM payments WHERE order_id = 4"), 0)
        self.assertEqual(self.count("SELECT COUNT(*) FROM addresses WHERE customer_id = 4"), 0)
        self.assertEqual(self.count("SELECT COUNT(*) FROM order_items"), 10)

    # --- parameterized queries -----------------------------------------------
    def test_parameterized_query_treats_input_as_data(self):
        attack = "x' OR '1'='1"
        self.assertEqual(self.db.execute("SELECT * FROM customers WHERE email = ?", (attack,)).fetchall(), [])

    # --- documented queries ----------------------------------------------------
    def test_every_documented_query_runs(self):
        for qid, title, _, sql in parse_queries((HERE / "queries.sql").read_text()):
            with self.subTest(qid=qid, title=title):
                self.db.execute(sql).fetchall()

    def test_q1_join_returns_every_order_line(self):
        self.assertEqual(len(self.query("Q1")), 12)

    def test_q5_lists_only_unordered_products(self):
        ordered = {row[0] for row in self.db.execute("SELECT DISTINCT product_id FROM order_items")}
        unordered = {row[0] for row in self.query("Q5")}
        self.assertEqual(unordered, set(range(1, 11)) - ordered)

    def test_q9_stored_totals_match_lines(self):
        self.assertEqual(self.query("Q9"), [])

    def test_q12_every_order_fully_paid(self):
        self.assertEqual(len(self.query("Q12")), 10)


if __name__ == "__main__":
    unittest.main()
