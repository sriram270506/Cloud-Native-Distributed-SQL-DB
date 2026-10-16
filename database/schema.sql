-- =====================================================================
-- Supermarket order database: complete DDL
-- Portable SQL; runs unchanged on SQLite 3.35+ and PostgreSQL 13+.
-- Run order: schema.sql -> seed.sql -> queries.sql
-- =====================================================================
-- SQLite enforces foreign keys only with this pragma. Delete this line when running on PostgreSQL.
PRAGMA foreign_keys = ON;

DROP VIEW  IF EXISTS order_summary;
DROP TABLE IF EXISTS payments;
DROP TABLE IF EXISTS order_items;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS products;
DROP TABLE IF EXISTS addresses;
DROP TABLE IF EXISTS customers;

-- ---------------------------------------------------------------------
-- CUSTOMERS: one row per registered shopper.
-- ---------------------------------------------------------------------
CREATE TABLE customers (
  customer_id  INTEGER      PRIMARY KEY,
  first_name   VARCHAR(50)  NOT NULL,
  last_name    VARCHAR(50)  NOT NULL,
  email        VARCHAR(120) NOT NULL UNIQUE,
  created_at   DATE         NOT NULL,
  updated_at   DATE         NOT NULL,
  status       VARCHAR(10)  NOT NULL DEFAULT 'active',
  CONSTRAINT chk_customer_email   CHECK (email LIKE '%_@_%._%'),
  CONSTRAINT chk_customer_status  CHECK (status IN ('active', 'inactive', 'suspended')),
  CONSTRAINT chk_customer_dates   CHECK (updated_at >= created_at)
);

-- ---------------------------------------------------------------------
-- ADDRESSES: a customer may keep several addresses, at most one per type.
-- Deleting a customer deletes their addresses.
-- ---------------------------------------------------------------------
CREATE TABLE addresses (
  address_id   INTEGER      PRIMARY KEY,
  customer_id  INTEGER      NOT NULL,
  line1        VARCHAR(100) NOT NULL,
  line2        VARCHAR(100),
  city         VARCHAR(60)  NOT NULL,
  state        CHAR(2)      NOT NULL,
  postal_code  VARCHAR(10)  NOT NULL,
  country      VARCHAR(60)  NOT NULL DEFAULT 'USA',
  type         VARCHAR(10)  NOT NULL,
  CONSTRAINT fk_address_customer FOREIGN KEY (customer_id)
    REFERENCES customers (customer_id) ON DELETE CASCADE,
  CONSTRAINT chk_address_type    CHECK (type IN ('home', 'work', 'billing', 'shipping')),
  CONSTRAINT uq_address_per_type UNIQUE (customer_id, type)
);

-- ---------------------------------------------------------------------
-- PRODUCTS: catalogue and current stock.
-- ---------------------------------------------------------------------
CREATE TABLE products (
  product_id    INTEGER        PRIMARY KEY,
  product_name  VARCHAR(100)   NOT NULL UNIQUE,
  category      VARCHAR(40)    NOT NULL,
  unit_price    NUMERIC(10, 2) NOT NULL,
  stock_qty     INTEGER        NOT NULL DEFAULT 0,
  created_at    DATE           NOT NULL,
  updated_at    DATE           NOT NULL,
  CONSTRAINT chk_product_price CHECK (unit_price > 0),
  CONSTRAINT chk_product_stock CHECK (stock_qty >= 0),
  CONSTRAINT chk_product_dates CHECK (updated_at >= created_at)
);

-- ---------------------------------------------------------------------
-- ORDERS: one purchase by one customer. Deleting a customer deletes their orders
-- (and, through further cascades, the order lines and payments).
-- ---------------------------------------------------------------------
CREATE TABLE orders (
  order_id      INTEGER        PRIMARY KEY,
  customer_id   INTEGER        NOT NULL,
  order_date    DATE           NOT NULL,
  total_amount  NUMERIC(10, 2) NOT NULL DEFAULT 0,
  status        VARCHAR(10)    NOT NULL DEFAULT 'pending',
  created_at    DATE           NOT NULL,
  updated_at    DATE           NOT NULL,
  CONSTRAINT fk_order_customer FOREIGN KEY (customer_id)
    REFERENCES customers (customer_id) ON DELETE CASCADE,
  CONSTRAINT chk_order_total   CHECK (total_amount >= 0),
  CONSTRAINT chk_order_status  CHECK (status IN ('pending', 'paid', 'shipped', 'completed', 'cancelled')),
  CONSTRAINT chk_order_dates   CHECK (updated_at >= created_at)
);

-- ---------------------------------------------------------------------
-- ORDER_ITEMS: resolves the M:N relationship between orders and products.
-- A product that appears on any order cannot be deleted (RESTRICT).
-- ---------------------------------------------------------------------
CREATE TABLE order_items (
  order_item_id  INTEGER        PRIMARY KEY,
  order_id       INTEGER        NOT NULL,
  product_id     INTEGER        NOT NULL,
  quantity       INTEGER        NOT NULL,
  unit_price     NUMERIC(10, 2) NOT NULL,
  line_total     NUMERIC(10, 2) NOT NULL,
  created_at     DATE           NOT NULL,
  CONSTRAINT fk_item_order   FOREIGN KEY (order_id)
    REFERENCES orders (order_id) ON DELETE CASCADE,
  CONSTRAINT fk_item_product FOREIGN KEY (product_id)
    REFERENCES products (product_id) ON DELETE RESTRICT,
  CONSTRAINT chk_item_quantity CHECK (quantity > 0),
  CONSTRAINT chk_item_price    CHECK (unit_price > 0),
  CONSTRAINT chk_item_total    CHECK (line_total = ROUND(quantity * unit_price, 2)),
  CONSTRAINT uq_item_product_per_order UNIQUE (order_id, product_id)
);

-- ---------------------------------------------------------------------
-- PAYMENTS: an order may be paid in one or more payments.
-- ---------------------------------------------------------------------
CREATE TABLE payments (
  payment_id       INTEGER        PRIMARY KEY,
  order_id         INTEGER        NOT NULL,
  method           VARCHAR(15)    NOT NULL,
  amount           NUMERIC(10, 2) NOT NULL,
  paid_at          DATE           NOT NULL,
  status           VARCHAR(10)    NOT NULL DEFAULT 'pending',
  transaction_ref  VARCHAR(30)    NOT NULL UNIQUE,
  CONSTRAINT fk_payment_order FOREIGN KEY (order_id)
    REFERENCES orders (order_id) ON DELETE CASCADE,
  CONSTRAINT chk_payment_method CHECK (method IN ('card', 'paypal', 'upi', 'cash', 'bank_transfer')),
  CONSTRAINT chk_payment_amount CHECK (amount > 0),
  CONSTRAINT chk_payment_status CHECK (status IN ('pending', 'paid', 'failed', 'refunded'))
);

-- ---------------------------------------------------------------------
-- Indexes on foreign keys and common filters (primary and unique keys are indexed automatically).
-- ---------------------------------------------------------------------
CREATE INDEX idx_addresses_customer  ON addresses (customer_id);
CREATE INDEX idx_orders_customer     ON orders (customer_id);
CREATE INDEX idx_orders_date         ON orders (order_date);
CREATE INDEX idx_order_items_product ON order_items (product_id);
CREATE INDEX idx_payments_order      ON payments (order_id);
CREATE INDEX idx_products_category   ON products (category);

-- ---------------------------------------------------------------------
-- View: one row per order with customer name, item count and amount paid.
-- ---------------------------------------------------------------------
CREATE VIEW order_summary AS
SELECT o.order_id,
       c.first_name || ' ' || c.last_name AS customer_name,
       o.order_date,
       o.status,
       o.total_amount,
       (SELECT COUNT(*)                   FROM order_items oi WHERE oi.order_id = o.order_id) AS item_lines,
       (SELECT COALESCE(SUM(p.amount), 0) FROM payments p     WHERE p.order_id  = o.order_id AND p.status = 'paid') AS amount_paid
FROM orders o
JOIN customers c ON c.customer_id = o.customer_id;
