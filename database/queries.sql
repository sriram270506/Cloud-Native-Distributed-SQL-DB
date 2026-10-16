-- =====================================================================
-- Advanced SQL queries for the e-commerce database.
-- Run after schema.sql and seed.sql. Each block starts with "-- @Qn" so
-- database/run_queries.py can run them one by one and record the output.
-- =====================================================================

-- @Q1 Multi-table join (4 tables): every order line with customer, product and order date
-- Purpose: a full invoice view; shows how normalized tables are recombined with joins.
SELECT o.order_id,
       c.first_name || ' ' || c.last_name AS customer,
       o.order_date,
       p.product_name,
       oi.quantity,
       oi.line_total
FROM orders o
JOIN customers   c  ON c.customer_id = o.customer_id
JOIN order_items oi ON oi.order_id   = o.order_id
JOIN products    p  ON p.product_id  = oi.product_id
ORDER BY o.order_id, p.product_name;

-- @Q2 GROUP BY with aggregates: revenue, units and distinct orders per category
-- Purpose: category-level sales report for the store manager.
SELECT p.category,
       COUNT(DISTINCT oi.order_id) AS orders,
       SUM(oi.quantity)            AS units_sold,
       ROUND(SUM(oi.line_total), 2) AS revenue,
       ROUND(AVG(oi.unit_price), 2) AS avg_unit_price
FROM order_items oi
JOIN products p ON p.product_id = oi.product_id
GROUP BY p.category
ORDER BY revenue DESC;

-- @Q3 GROUP BY ... HAVING: categories whose revenue exceeds 100
-- Purpose: HAVING filters groups after aggregation, unlike WHERE which filters rows.
SELECT p.category, ROUND(SUM(oi.line_total), 2) AS revenue
FROM order_items oi
JOIN products p ON p.product_id = oi.product_id
GROUP BY p.category
HAVING SUM(oi.line_total) > 100
ORDER BY revenue DESC;

-- @Q4 Correlated subquery: customers who spent more than the average order value
-- Purpose: the inner query is evaluated against the outer customer row.
SELECT c.customer_id,
       c.first_name || ' ' || c.last_name AS customer,
       (SELECT ROUND(SUM(o.total_amount), 2) FROM orders o WHERE o.customer_id = c.customer_id) AS total_spent
FROM customers c
WHERE (SELECT SUM(o.total_amount) FROM orders o WHERE o.customer_id = c.customer_id)
      > (SELECT AVG(total_amount) FROM orders)
ORDER BY total_spent DESC;

-- @Q5 NOT EXISTS subquery: products that have never been ordered
-- Purpose: anti-join to find dead stock.
SELECT p.product_id, p.product_name, p.category, p.stock_qty
FROM products p
WHERE NOT EXISTS (SELECT 1 FROM order_items oi WHERE oi.product_id = p.product_id)
ORDER BY p.product_id;

-- @Q6 IN subquery: customers who paid at least once with PayPal
-- Purpose: a nested query across three tables without an explicit join.
SELECT customer_id, first_name, last_name, email
FROM customers
WHERE customer_id IN (
  SELECT o.customer_id
  FROM orders o
  WHERE o.order_id IN (SELECT order_id FROM payments WHERE method = 'paypal')
)
ORDER BY customer_id;

-- @Q7 LEFT JOIN + aggregate: order count and spend for every customer, including those with none
-- Purpose: outer join keeps unmatched rows; COALESCE turns NULL into 0.
SELECT c.customer_id,
       c.first_name,
       COUNT(o.order_id)                    AS orders,
       COALESCE(ROUND(SUM(o.total_amount), 2), 0) AS spent
FROM customers c
LEFT JOIN orders o ON o.customer_id = c.customer_id
GROUP BY c.customer_id, c.first_name
ORDER BY spent DESC, c.customer_id;

-- @Q8 Common table expression + window function: rank products by revenue within each category
-- Purpose: RANK() OVER (PARTITION BY ...) computes a per-group ranking without collapsing rows.
WITH product_revenue AS (
  SELECT p.category, p.product_name, COALESCE(SUM(oi.line_total), 0) AS revenue
  FROM products p
  LEFT JOIN order_items oi ON oi.product_id = p.product_id
  GROUP BY p.category, p.product_name
)
SELECT category,
       product_name,
       ROUND(revenue, 2) AS revenue,
       RANK() OVER (PARTITION BY category ORDER BY revenue DESC) AS rank_in_category
FROM product_revenue
ORDER BY category, rank_in_category;

-- @Q9 Data-integrity check: orders whose stored total differs from the sum of their lines
-- Purpose: total_amount is a stored derived value; this query proves it is consistent (expect 0 rows).
SELECT o.order_id, o.total_amount, ROUND(SUM(oi.line_total), 2) AS computed_total
FROM orders o
JOIN order_items oi ON oi.order_id = o.order_id
GROUP BY o.order_id, o.total_amount
HAVING ROUND(SUM(oi.line_total), 2) <> o.total_amount;

-- @Q10 CASE expression: classify customers into spending tiers
-- Purpose: conditional logic inside SELECT for segmentation.
SELECT c.customer_id,
       c.first_name || ' ' || c.last_name AS customer,
       ROUND(SUM(o.total_amount), 2) AS spent,
       CASE
         WHEN SUM(o.total_amount) >= 200 THEN 'gold'
         WHEN SUM(o.total_amount) >= 50  THEN 'silver'
         ELSE 'bronze'
       END AS tier
FROM customers c
JOIN orders o ON o.customer_id = c.customer_id
GROUP BY c.customer_id, c.first_name, c.last_name
ORDER BY spent DESC;

-- @Q11 Scalar subquery in SELECT: each product's price compared to its category average
-- Purpose: per-row comparison against a group statistic.
SELECT p.product_name,
       p.category,
       p.unit_price,
       ROUND((SELECT AVG(p2.unit_price) FROM products p2 WHERE p2.category = p.category), 2) AS category_avg,
       ROUND(p.unit_price - (SELECT AVG(p2.unit_price) FROM products p2 WHERE p2.category = p.category), 2) AS diff_from_avg
FROM products p
ORDER BY p.category, p.unit_price DESC;

-- @Q12 View usage: orders that are fully paid, read through the order_summary view
-- Purpose: views hide join/subquery complexity behind a virtual table.
SELECT order_id, customer_name, total_amount, amount_paid, item_lines
FROM order_summary
WHERE amount_paid >= total_amount
ORDER BY order_id;

-- @Q13 UNION: one contact list of cities from home and work addresses, labelled by type
-- Purpose: combine two result sets with the same shape.
SELECT city, state, 'home' AS address_type FROM addresses WHERE type = 'home'
UNION
SELECT city, state, 'work' AS address_type FROM addresses WHERE type = 'work'
ORDER BY address_type, city;

-- @Q14 Date filtering + aggregates: daily revenue in the first week of June 2024
-- Purpose: range predicate on a DATE column with BETWEEN.
SELECT order_date, COUNT(*) AS orders, ROUND(SUM(total_amount), 2) AS revenue
FROM orders
WHERE order_date BETWEEN '2024-06-01' AND '2024-06-07'
GROUP BY order_date
ORDER BY order_date;
