# Supermarket relational schema

The business database is a normalized order and inventory model. Each table
stores one subject, and relationships are represented by foreign keys.

| Table | Primary key | Foreign keys | Purpose |
| --- | --- | --- | --- |
| `customers` | `customer_id` | — | Customer identity and account status |
| `addresses` | `address_id` | `customer_id -> customers.customer_id` | Customer delivery or work addresses |
| `products` | `product_id` | — | Product catalogue and stock |
| `orders` | `order_id` | `customer_id -> customers.customer_id` | Customer purchases |
| `order_items` | `order_item_id` | `order_id -> orders.order_id`, `product_id -> products.product_id` | Products and quantities in each order |
| `payments` | `payment_id` | `order_id -> orders.order_id` | Payment records for orders |

## Functional dependencies

- `customer_id -> first_name, last_name, email, created_at, updated_at, status`
- `address_id -> customer_id, line1, line2, city, state, postal_code, country, type`
- `product_id -> product_name, category, unit_price, stock_qty, created_at, updated_at`
- `order_id -> customer_id, order_date, total_amount, status, created_at, updated_at`
- `order_item_id -> order_id, product_id, quantity, unit_price, line_total, created_at`
- `payment_id -> order_id, method, amount, paid_at, status, transaction_ref`

## Normalization

- **1NF:** Every column contains one atomic value; repeating order products are
  stored as separate `order_items` rows.
- **2NF:** Non-key attributes depend on the complete key. Product details are
  not repeated in `orders`; they are stored in `products` and referenced by
  `order_items`.
- **3NF:** Non-key attributes do not determine other non-key attributes.
  Customer address details are separated from customer identity, and payment
  details are separated from orders.

The SQL engine currently stores values as strings internally for its compact
educational implementation, while this schema documents the intended logical
types and keys.

## Sample records

The matching sample records are stored in
`data/sample_db/` as CSV files:

- `customers.csv`
- `addresses.csv`
- `products.csv`
- `orders.csv`
- `order_items.csv`
- `payments.csv`
