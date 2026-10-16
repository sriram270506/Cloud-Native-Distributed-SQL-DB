-- Sample records (10-12 rows per table), generated from data/sample_db/*.csv.
-- Run after schema.sql. Parents are inserted before children so every foreign key resolves.
BEGIN;

-- customers
INSERT INTO customers (customer_id, first_name, last_name, email, created_at, updated_at, status) VALUES (1, 'Emma', 'Johnson', 'emma.johnson@example.com', '2024-01-10', '2024-06-01', 'active');
INSERT INTO customers (customer_id, first_name, last_name, email, created_at, updated_at, status) VALUES (2, 'Liam', 'Smith', 'liam.smith@example.com', '2024-02-12', '2024-06-02', 'active');
INSERT INTO customers (customer_id, first_name, last_name, email, created_at, updated_at, status) VALUES (3, 'Olivia', 'Williams', 'olivia.williams@example.com', '2024-03-05', '2024-06-03', 'active');
INSERT INTO customers (customer_id, first_name, last_name, email, created_at, updated_at, status) VALUES (4, 'Noah', 'Brown', 'noah.brown@example.com', '2024-03-20', '2024-06-04', 'active');
INSERT INTO customers (customer_id, first_name, last_name, email, created_at, updated_at, status) VALUES (5, 'Ava', 'Jones', 'ava.jones@example.com', '2024-04-01', '2024-06-05', 'active');
INSERT INTO customers (customer_id, first_name, last_name, email, created_at, updated_at, status) VALUES (6, 'William', 'Garcia', 'william.garcia@example.com', '2024-04-15', '2024-06-06', 'active');
INSERT INTO customers (customer_id, first_name, last_name, email, created_at, updated_at, status) VALUES (7, 'Sophia', 'Miller', 'sophia.miller@example.com', '2024-05-02', '2024-06-07', 'active');
INSERT INTO customers (customer_id, first_name, last_name, email, created_at, updated_at, status) VALUES (8, 'James', 'Davis', 'james.davis@example.com', '2024-05-10', '2024-06-08', 'active');
INSERT INTO customers (customer_id, first_name, last_name, email, created_at, updated_at, status) VALUES (9, 'Isabella', 'Martinez', 'isabella.martinez@example.com', '2024-05-20', '2024-06-09', 'active');
INSERT INTO customers (customer_id, first_name, last_name, email, created_at, updated_at, status) VALUES (10, 'Logan', 'Hernandez', 'logan.hernandez@example.com', '2024-06-01', '2024-06-10', 'active');

-- addresses
INSERT INTO addresses (address_id, customer_id, line1, line2, city, state, postal_code, country, type) VALUES (1, 1, '123 Maple St', NULL, 'Springfield', 'IL', '62701', 'USA', 'home');
INSERT INTO addresses (address_id, customer_id, line1, line2, city, state, postal_code, country, type) VALUES (2, 2, '456 Oak Ave', NULL, 'Madison', 'WI', '53703', 'USA', 'home');
INSERT INTO addresses (address_id, customer_id, line1, line2, city, state, postal_code, country, type) VALUES (3, 3, '789 Pine Rd', NULL, 'Columbus', 'OH', '43085', 'USA', 'home');
INSERT INTO addresses (address_id, customer_id, line1, line2, city, state, postal_code, country, type) VALUES (4, 4, '12 Birch Ln', NULL, 'Austin', 'TX', '73301', 'USA', 'home');
INSERT INTO addresses (address_id, customer_id, line1, line2, city, state, postal_code, country, type) VALUES (5, 5, '34 Cedar Blvd', NULL, 'Denver', 'CO', '80014', 'USA', 'home');
INSERT INTO addresses (address_id, customer_id, line1, line2, city, state, postal_code, country, type) VALUES (6, 6, '56 Elm St', NULL, 'Seattle', 'WA', '98101', 'USA', 'work');
INSERT INTO addresses (address_id, customer_id, line1, line2, city, state, postal_code, country, type) VALUES (7, 7, '78 Aspen Way', NULL, 'Portland', 'OR', '97201', 'USA', 'home');
INSERT INTO addresses (address_id, customer_id, line1, line2, city, state, postal_code, country, type) VALUES (8, 8, '90 Walnut Dr', NULL, 'San Diego', 'CA', '92101', 'USA', 'work');
INSERT INTO addresses (address_id, customer_id, line1, line2, city, state, postal_code, country, type) VALUES (9, 9, '101 Cherry Ct', NULL, 'Raleigh', 'NC', '27601', 'USA', 'home');
INSERT INTO addresses (address_id, customer_id, line1, line2, city, state, postal_code, country, type) VALUES (10, 10, '202 Poplar Pl', NULL, 'Orlando', 'FL', '32801', 'USA', 'home');

-- products
INSERT INTO products (product_id, product_name, category, unit_price, stock_qty, created_at, updated_at) VALUES (1, 'Widget A', 'widgets', 19.99, 100, '2024-01-01', '2024-06-01');
INSERT INTO products (product_id, product_name, category, unit_price, stock_qty, created_at, updated_at) VALUES (2, 'Widget B', 'widgets', 29.99, 80, '2024-01-05', '2024-06-02');
INSERT INTO products (product_id, product_name, category, unit_price, stock_qty, created_at, updated_at) VALUES (3, 'Gadget X', 'gadgets', 9.99, 200, '2024-02-01', '2024-06-03');
INSERT INTO products (product_id, product_name, category, unit_price, stock_qty, created_at, updated_at) VALUES (4, 'Gadget Y', 'gadgets', 14.99, 150, '2024-02-10', '2024-06-04');
INSERT INTO products (product_id, product_name, category, unit_price, stock_qty, created_at, updated_at) VALUES (5, 'Tool Alpha', 'tools', 49.99, 50, '2024-03-01', '2024-06-05');
INSERT INTO products (product_id, product_name, category, unit_price, stock_qty, created_at, updated_at) VALUES (6, 'Tool Beta', 'tools', 59.99, 40, '2024-03-15', '2024-06-06');
INSERT INTO products (product_id, product_name, category, unit_price, stock_qty, created_at, updated_at) VALUES (7, 'Accessory 1', 'accessories', 4.99, 500, '2024-04-01', '2024-06-07');
INSERT INTO products (product_id, product_name, category, unit_price, stock_qty, created_at, updated_at) VALUES (8, 'Accessory 2', 'accessories', 6.99, 400, '2024-04-10', '2024-06-08');
INSERT INTO products (product_id, product_name, category, unit_price, stock_qty, created_at, updated_at) VALUES (9, 'Device One', 'devices', 199.99, 20, '2024-05-01', '2024-06-09');
INSERT INTO products (product_id, product_name, category, unit_price, stock_qty, created_at, updated_at) VALUES (10, 'Device Two', 'devices', 299.99, 10, '2024-05-15', '2024-06-10');

-- orders
INSERT INTO orders (order_id, customer_id, order_date, total_amount, status, created_at, updated_at) VALUES (1, 1, '2024-06-01', 29.97, 'completed', '2024-06-01', '2024-06-02');
INSERT INTO orders (order_id, customer_id, order_date, total_amount, status, created_at, updated_at) VALUES (2, 2, '2024-06-02', 29.99, 'completed', '2024-06-02', '2024-06-03');
INSERT INTO orders (order_id, customer_id, order_date, total_amount, status, created_at, updated_at) VALUES (3, 3, '2024-06-03', 9.99, 'completed', '2024-06-03', '2024-06-04');
INSERT INTO orders (order_id, customer_id, order_date, total_amount, status, created_at, updated_at) VALUES (4, 4, '2024-06-04', 204.98, 'completed', '2024-06-04', '2024-06-05');
INSERT INTO orders (order_id, customer_id, order_date, total_amount, status, created_at, updated_at) VALUES (5, 5, '2024-06-05', 14.99, 'completed', '2024-06-05', '2024-06-06');
INSERT INTO orders (order_id, customer_id, order_date, total_amount, status, created_at, updated_at) VALUES (6, 6, '2024-06-06', 99.98, 'completed', '2024-06-06', '2024-06-07');
INSERT INTO orders (order_id, customer_id, order_date, total_amount, status, created_at, updated_at) VALUES (7, 7, '2024-06-07', 6.99, 'completed', '2024-06-07', '2024-06-08');
INSERT INTO orders (order_id, customer_id, order_date, total_amount, status, created_at, updated_at) VALUES (8, 8, '2024-06-08', 199.99, 'completed', '2024-06-08', '2024-06-09');
INSERT INTO orders (order_id, customer_id, order_date, total_amount, status, created_at, updated_at) VALUES (9, 9, '2024-06-09', 299.99, 'completed', '2024-06-09', '2024-06-10');
INSERT INTO orders (order_id, customer_id, order_date, total_amount, status, created_at, updated_at) VALUES (10, 10, '2024-06-10', 49.99, 'completed', '2024-06-10', '2024-06-11');

-- order_items
INSERT INTO order_items (order_item_id, order_id, product_id, quantity, unit_price, line_total, created_at) VALUES (1, 1, 1, 1, 19.99, 19.99, '2024-06-01');
INSERT INTO order_items (order_item_id, order_id, product_id, quantity, unit_price, line_total, created_at) VALUES (2, 1, 7, 2, 4.99, 9.98, '2024-06-01');
INSERT INTO order_items (order_item_id, order_id, product_id, quantity, unit_price, line_total, created_at) VALUES (3, 2, 2, 1, 29.99, 29.99, '2024-06-02');
INSERT INTO order_items (order_item_id, order_id, product_id, quantity, unit_price, line_total, created_at) VALUES (4, 3, 3, 1, 9.99, 9.99, '2024-06-03');
INSERT INTO order_items (order_item_id, order_id, product_id, quantity, unit_price, line_total, created_at) VALUES (5, 4, 9, 1, 199.99, 199.99, '2024-06-04');
INSERT INTO order_items (order_item_id, order_id, product_id, quantity, unit_price, line_total, created_at) VALUES (6, 4, 7, 1, 4.99, 4.99, '2024-06-04');
INSERT INTO order_items (order_item_id, order_id, product_id, quantity, unit_price, line_total, created_at) VALUES (7, 5, 4, 1, 14.99, 14.99, '2024-06-05');
INSERT INTO order_items (order_item_id, order_id, product_id, quantity, unit_price, line_total, created_at) VALUES (8, 6, 5, 2, 49.99, 99.98, '2024-06-06');
INSERT INTO order_items (order_item_id, order_id, product_id, quantity, unit_price, line_total, created_at) VALUES (9, 7, 8, 1, 6.99, 6.99, '2024-06-07');
INSERT INTO order_items (order_item_id, order_id, product_id, quantity, unit_price, line_total, created_at) VALUES (10, 9, 10, 1, 299.99, 299.99, '2024-06-09');
INSERT INTO order_items (order_item_id, order_id, product_id, quantity, unit_price, line_total, created_at) VALUES (11, 8, 9, 1, 199.99, 199.99, '2024-06-08');
INSERT INTO order_items (order_item_id, order_id, product_id, quantity, unit_price, line_total, created_at) VALUES (12, 10, 5, 1, 49.99, 49.99, '2024-06-10');

-- payments
INSERT INTO payments (payment_id, order_id, method, amount, paid_at, status, transaction_ref) VALUES (1, 1, 'card', 29.97, '2024-06-01', 'paid', 'tx1001');
INSERT INTO payments (payment_id, order_id, method, amount, paid_at, status, transaction_ref) VALUES (2, 2, 'card', 29.99, '2024-06-02', 'paid', 'tx1002');
INSERT INTO payments (payment_id, order_id, method, amount, paid_at, status, transaction_ref) VALUES (3, 3, 'paypal', 9.99, '2024-06-03', 'paid', 'tx1003');
INSERT INTO payments (payment_id, order_id, method, amount, paid_at, status, transaction_ref) VALUES (4, 4, 'card', 204.98, '2024-06-04', 'paid', 'tx1004');
INSERT INTO payments (payment_id, order_id, method, amount, paid_at, status, transaction_ref) VALUES (5, 5, 'card', 14.99, '2024-06-05', 'paid', 'tx1005');
INSERT INTO payments (payment_id, order_id, method, amount, paid_at, status, transaction_ref) VALUES (6, 6, 'card', 99.98, '2024-06-06', 'paid', 'tx1006');
INSERT INTO payments (payment_id, order_id, method, amount, paid_at, status, transaction_ref) VALUES (7, 7, 'paypal', 6.99, '2024-06-07', 'paid', 'tx1007');
INSERT INTO payments (payment_id, order_id, method, amount, paid_at, status, transaction_ref) VALUES (8, 8, 'card', 199.99, '2024-06-08', 'paid', 'tx1008');
INSERT INTO payments (payment_id, order_id, method, amount, paid_at, status, transaction_ref) VALUES (9, 9, 'card', 299.99, '2024-06-09', 'paid', 'tx1009');
INSERT INTO payments (payment_id, order_id, method, amount, paid_at, status, transaction_ref) VALUES (10, 10, 'card', 49.99, '2024-06-10', 'paid', 'tx1010');

COMMIT;
