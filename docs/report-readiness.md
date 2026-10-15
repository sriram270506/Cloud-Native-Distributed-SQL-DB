# DBMS project report readiness

This checklist maps the required report sections to evidence in the project.

| Required section | Project evidence | Status |
| --- | --- | --- |
| Project title | `README.md` and `ARCHITECTURE.md` | Complete |
| Team members | Supplied by the project team for the presentation | Needs names |
| Introduction and problem statement | `README.md`, `ARCHITECTURE.md` | Complete |
| Conceptual and logical database design | `docs/relational-schema.md`, `docs/supermarket_er.mmd` | Complete |
| Normalization and functional dependencies | `docs/relational-schema.md` | Complete |
| Advanced SQL implementation and query design | `sql/engine.go`, `sql/engine_test.go`, frontend query console | Complete for the supported SQL scope |
| ER diagram and UML diagram | `docs/supermarket_er.mmd`, `docs/supermarket_uml.mmd` | Complete |
| Relational schema | `sql/ecom_schema.sql`, `docs/relational-schema.md` | Complete |
| Database tables, attributes, keys, and sample records | `sql/ecom_schema.sql`, `data/sample_db/*.csv` | Complete |
| Conclusion and future scope | `README.md`, `ARCHITECTURE.md` | Complete |

## Implemented database scope

The running database contains:

- `customers`
- `addresses`
- `products`
- `orders`
- `order_items`
- `payments`

Each table currently has 10 sample records in the Docker-backed Raft database.

The SQL layer supports CRUD, `WHERE`, `AND`, `OR`, `LIKE`, comparison
operators, two-table inner equi-`JOIN`, `GROUP BY`, `HAVING`, `COUNT`, `SUM`,
`AVG`, `MIN`, `MAX`, `ORDER BY`, `LIMIT`, `OFFSET`, and `DROP TABLE`.

## Important implementation boundary

The schema file documents primary keys, foreign keys, unique fields, and
logical data types for the report. The educational SQL engine currently serializes row values as strings and
enforces the first column as the row key; it does not yet enforce every SQL
constraint or support subqueries.
These are appropriate future-scope items to state in the presentation rather
than claiming production-level relational enforcement.
