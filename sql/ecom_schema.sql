-- Supermarket relational schema used by the sample dataset.
-- The application stores the corresponding tables as customers, addresses,
-- products, orders, order_items, and payments.

CREATE TABLE customers (
  customer_id INTEGER PRIMARY KEY,
  first_name TEXT,
  last_name TEXT,
  email TEXT UNIQUE,
  created_at DATE,
  updated_at DATE,
  status TEXT
);

CREATE TABLE addresses (
  address_id INTEGER PRIMARY KEY,
  customer_id INTEGER REFERENCES customers(customer_id),
  line1 TEXT,
  line2 TEXT,
  city TEXT,
  state TEXT,
  postal_code TEXT,
  country TEXT,
  type TEXT
);

CREATE TABLE products (
  product_id INTEGER PRIMARY KEY,
  product_name TEXT,
  category TEXT,
  unit_price REAL,
  stock_qty INTEGER,
  created_at DATE,
  updated_at DATE
);

CREATE TABLE orders (
  order_id INTEGER PRIMARY KEY,
  customer_id INTEGER REFERENCES customers(customer_id),
  order_date DATE,
  total_amount REAL,
  status TEXT,
  created_at DATE,
  updated_at DATE
);

CREATE TABLE order_items (
  order_item_id INTEGER PRIMARY KEY,
  order_id INTEGER REFERENCES orders(order_id),
  product_id INTEGER REFERENCES products(product_id),
  quantity INTEGER,
  unit_price REAL,
  line_total REAL,
  created_at DATE
);

CREATE TABLE payments (
  payment_id INTEGER PRIMARY KEY,
  order_id INTEGER REFERENCES orders(order_id),
  method TEXT,
  amount REAL,
  paid_at DATE,
  status TEXT,
  transaction_ref TEXT
);
