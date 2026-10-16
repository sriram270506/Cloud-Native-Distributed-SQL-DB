"""Build the project report (HTML + PDF) from the repository's own artifacts.

    python docs/report/build_report.py

Reads docs/report/team.json, database/*.sql, data/sample_db/*.csv, docs/*.mmd,
database/results/*.md and docs/screenshots/*.png, writes docs/report/report.html, then
prints it to docs/report/Project_Report.pdf with headless Chrome or Edge.
Edit team.json (names, batch, group) and re-run to regenerate the PDF.
"""
import base64
import csv
import html
import json
import os
import shutil
import sqlite3
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
REPORT = ROOT / "docs" / "report"
sys.path.insert(0, str(ROOT / "database"))
from run_queries import build, parse_queries  # noqa: E402

TABLES = ["customers", "addresses", "products", "orders", "order_items", "payments"]
e = html.escape


def table_html(columns, rows, cls=""):
    head = "".join(f"<th>{e(str(c))}</th>" for c in columns)
    body = "".join("<tr>" + "".join(f"<td>{e('' if v is None else str(v))}</td>" for v in row) + "</tr>" for row in rows)
    return f'<table class="{cls}"><thead><tr>{head}</tr></thead><tbody>{body}</tbody></table>'


def mermaid(name):
    return f'<pre class="mermaid">{e((ROOT / "docs" / name).read_text())}</pre>'


def image(name, caption):
    data = base64.b64encode((ROOT / "docs" / "screenshots" / name).read_bytes()).decode()
    return f'<figure><img src="data:image/png;base64,{data}"><figcaption>{e(caption)}</figcaption></figure>'


def md_to_html(text):
    """Tiny converter for the generated result files: headings, tables, code, paragraphs."""
    out, lines, i = [], text.splitlines(), 0
    while i < len(lines):
        line = lines[i]
        if line.startswith("```"):
            block = []
            i += 1
            while i < len(lines) and not lines[i].startswith("```"):
                block.append(lines[i])
                i += 1
            out.append(f"<pre><code>{e(chr(10).join(block))}</code></pre>")
        elif line.startswith("|"):
            rows = []
            while i < len(lines) and lines[i].startswith("|"):
                if not set(lines[i]) <= set("|-: "):
                    rows.append([c.strip() for c in lines[i].strip("|").split("|")])
                i += 1
            out.append(table_html(rows[0], rows[1:], "small"))
            continue
        elif line.startswith("## "):
            out.append(f"<h4>{e(line[3:])}</h4>")
        elif line.startswith("# "):
            pass
        elif line.strip():
            out.append(f"<p>{e(line).replace('`', '')}</p>")
        i += 1
    return "\n".join(out)


def schema_details(db):
    """Per-table column structure with keys and constraints, read back from SQLite."""
    sql_by_table = {name: sql for name, sql in db.execute("SELECT name, sql FROM sqlite_master WHERE type='table'")}
    sections = []
    for table in TABLES:
        fks = {row[3]: f"FK → {row[2]}({row[4]}) ON DELETE {row[6]}" for row in db.execute(f"PRAGMA foreign_key_list({table})")}
        uniques = set()
        for index in db.execute(f"PRAGMA index_list({table})"):
            if index[2]:
                cols = [c[2] for c in db.execute(f"PRAGMA index_info({index[1]})")]
                if len(cols) == 1:
                    uniques.add(cols[0])
        rows = []
        for cid, name, ctype, notnull, default, pk in db.execute(f"PRAGMA table_info({table})"):
            keys = []
            if pk:
                keys.append("PK")
            if name in fks:
                keys.append(fks[name])
            if name in uniques:
                keys.append("UNIQUE")
            rows.append([name, ctype, "NOT NULL" if notnull or pk else "NULL", default or "", ", ".join(keys)])
        checks = [line.strip().rstrip(",") for line in sql_by_table[table].splitlines()
                  if "CHECK" in line or ("UNIQUE (" in line and "CONSTRAINT" in line)]
        with open(ROOT / "data" / "sample_db" / f"{table}.csv", newline="") as handle:
            sample = list(csv.reader(handle))
        sections.append(f"""
<h3>9.{TABLES.index(table) + 1} {table}</h3>
{table_html(["Attribute", "Type", "Null", "Default", "Keys"], rows)}
<p class="label">Table-level constraints</p>
<pre><code>{e(chr(10).join(checks))}</code></pre>
<p class="label">Sample records ({len(sample) - 1} rows)</p>
{table_html(sample[0], sample[1:], "small")}""")
    return "\n".join(sections)


def query_section(db):
    parts = []
    for qid, title, purpose, sql in parse_queries((ROOT / "database" / "queries.sql").read_text()):
        cursor = db.execute(sql)
        columns = [d[0] for d in cursor.description]
        rows = cursor.fetchall()
        result = table_html(columns, rows, "small") if rows else "<p><em>No rows (expected).</em></p>"
        parts.append(f"""<div class="query"><h4>{qid}. {e(title)}</h4><p>{e(purpose)}</p>
<pre><code>{e(sql)}</code></pre><p class="label">Output ({len(rows)} rows)</p>{result}</div>""")
    return "\n".join(parts)


def main():
    team = json.loads((REPORT / "team.json").read_text())
    db = sqlite3.connect(":memory:")
    build(db)
    members = table_html(["#", "Name", "Roll number"], [[i + 1, m["name"], m["roll_no"]] for i, m in enumerate(team["members"])])
    counts = {t: db.execute(f"SELECT COUNT(*) FROM {t}").fetchone()[0] for t in TABLES}
    results = ROOT / "database" / "results"

    page = f"""<!doctype html><html><head><meta charset="utf-8"><title>Project Report</title>
<style>
@page {{ size: A4; margin: 18mm 16mm; }}
body {{ font-family: "Segoe UI", Arial, sans-serif; font-size: 10.5pt; line-height: 1.45; color: #1d2329; }}
h1 {{ font-size: 24pt; margin: 0 0 8px; }}
h2 {{ font-size: 16pt; border-bottom: 2px solid #1f6f50; padding-bottom: 4px; margin-top: 0; page-break-before: always; color: #174f3a; }}
h3 {{ font-size: 12.5pt; margin: 18px 0 6px; color: #174f3a; }}
h4 {{ font-size: 11pt; margin: 14px 0 4px; }}
table {{ border-collapse: collapse; width: 100%; margin: 6px 0 10px; font-size: 9pt; page-break-inside: auto; }}
th, td {{ border: 1px solid #c9d1d6; padding: 3px 6px; text-align: left; vertical-align: top; }}
th {{ background: #e8f1ec; }}
table.small {{ font-size: 8pt; }}
tr {{ page-break-inside: avoid; }}
pre {{ background: #f5f7f8; border: 1px solid #dde3e6; padding: 6px 8px; font-size: 8.3pt; white-space: pre-wrap; page-break-inside: avoid; }}
pre.mermaid {{ background: none; border: none; text-align: center; }}
.label {{ font-weight: 600; margin: 8px 0 2px; font-size: 9.5pt; }}
.query {{ page-break-inside: auto; }}
figure {{ margin: 10px 0; text-align: center; page-break-inside: avoid; }}
figure img {{ max-width: 100%; border: 1px solid #c9d1d6; }}
figcaption {{ font-size: 9pt; color: #555; }}
.title-page {{ text-align: center; padding-top: 140px; }}
.title-page .course {{ font-size: 13pt; color: #444; margin-top: 30px; }}
.title-page .meta {{ margin-top: 40px; font-size: 11pt; }}
.toc li {{ margin: 3px 0; }}
</style></head><body>

<section class="title-page">
  <p class="course">{e(team["course"])} &mdash; Project Report</p>
  <h1>{e(team["project_name"])}</h1>
  <p class="meta">Batch {e(team["batch"])} &middot; Group {e(team["group"])}</p>
  <p class="meta">GitHub repository: <a href="{e(team["github"])}">{e(team["github"])}</a></p>
  <h3 style="margin-top:50px">Contents</h3>
  <ol class="toc" style="text-align:left; display:inline-block">
    <li>Project Title</li><li>Team Members</li><li>Introduction and Problem Statement</li>
    <li>Conceptual and Logical Database Design</li><li>Normalization and Functional Dependencies</li>
    <li>Advanced SQL Implementation and Query Design</li><li>ER Diagram and UML Diagram</li>
    <li>Relational Schema</li><li>Database Tables</li><li>Conclusion and Future Scope</li>
    <li>Appendix A: Testing and Validation</li><li>Appendix B: Setup and Demonstration</li>
  </ol>
</section>

<h2>1. Project Title</h2>
<p><strong>{e(team["project_name"])}</strong></p>
<p>The project has two layers:</p>
<ul>
  <li><strong>A distributed database engine.</strong> Every node is a separate process. Writes are replicated with the Raft consensus protocol and run as MVCC transactions. The engine is written in Go and deploys on Docker Compose or Kubernetes.</li>
  <li><strong>A supermarket order-management database.</strong> It is designed, normalized and queried in standard SQL, and it runs on that engine.</li>
</ul>
<p>GitHub repository: <a href="{e(team["github"])}">{e(team["github"])}</a></p>

<h2>2. Team Members</h2>
<p>Batch: {e(team["batch"])} &nbsp; Group: {e(team["group"])}</p>
{members}

<h2>3. Introduction and Problem Statement</h2>
<h3>3.1 Introduction</h3>
<p>A traditional relational database runs on one server. If that server fails, the data is unavailable and recent writes may be lost. NewSQL systems such as Google Spanner, CockroachDB and TiDB keep the SQL interface and ACID transactions, but spread the data across several machines. They agree on every change with a consensus protocol.</p>
<p>This project builds a small database of that kind from first principles:</p>
<ul>
  <li><strong>Raft consensus</strong> elects a leader and replicates each write to a majority of nodes before it is acknowledged.</li>
  <li><strong>A write-ahead log</strong> plus a durable Raft log let a crashed node recover its state.</li>
  <li><strong>MVCC</strong> (multi-version concurrency control) gives transactions snapshot isolation.</li>
  <li><strong>A SQL layer</strong> supports DDL, DML, joins, grouping and aggregates.</li>
  <li><strong>An HTTP API and a React console</strong> let users run SQL against any node.</li>
</ul>
<h3>3.2 Problem statement</h3>
<p>A supermarket needs one consistent record of its customers, addresses, products, orders, order lines and payments. The record must:</p>
<ol>
  <li>stay available and correct when a server fails, with no lost acknowledged writes and no split-brain;</li>
  <li>apply multi-statement updates atomically, for example creating an order with its lines and payment;</li>
  <li>enforce data integrity through keys and constraints, and stay free of update anomalies through normalization;</li>
  <li>answer analytical questions such as revenue per category, top customers and unsold stock with SQL;</li>
  <li>accept client input safely, through parameterized queries, so SQL injection is not possible.</li>
</ol>
<h3>3.3 Objectives</h3>
<ul>
  <li>Design a normalized (3NF/BCNF) relational schema with full PK, FK, UNIQUE, CHECK and ON DELETE CASCADE constraints.</li>
  <li>Implement a replicated SQL database: Raft over TCP, a durable BoltDB log, linearizable reads and automatic failover.</li>
  <li>Implement MVCC transactions and all-or-nothing batches that are replicated in the same order on every node.</li>
  <li>Demonstrate advanced SQL (multi-table joins, correlated and nested subqueries, GROUP BY/HAVING, window functions, views) and validate everything with automated tests.</li>
</ul>

<h2>4. Conceptual and Logical Database Design</h2>
<h3>4.1 Conceptual design: entities and relationships</h3>
{table_html(["Entity", "Description", "Key attribute"], [
    ["Customer", "A registered shopper", "customer_id"],
    ["Address", "A postal address owned by a customer (home, work, billing or shipping)", "address_id"],
    ["Product", "An item in the catalogue, with price and stock", "product_id"],
    ["Order", "One purchase by one customer on a date", "order_id"],
    ["Order Item", "One product line within an order (associative entity)", "order_item_id"],
    ["Payment", "A payment made against an order", "payment_id"],
])}
{table_html(["Relationship", "Cardinality", "Participation"], [
    ["Customer owns Address", "1 : N", "An address must belong to a customer (total); a customer may have none (partial)"],
    ["Customer places Order", "1 : N", "Every order has exactly one customer"],
    ["Order contains Product (through Order Item)", "M : N, resolved to two 1 : N", "An order has at least one line; a product may be on no order"],
    ["Order is settled by Payment", "1 : N", "A payment belongs to exactly one order"],
])}
<h3>4.2 Logical design: from ER model to relations</h3>
<ul>
  <li>Each strong entity becomes a relation, and its key becomes the primary key.</li>
  <li>Each 1:N relationship becomes a foreign key on the N side: <code>addresses.customer_id</code>, <code>orders.customer_id</code> and <code>payments.order_id</code>.</li>
  <li>The M:N Order–Product relationship becomes the associative relation <code>order_items</code>. It has two foreign keys and a <code>UNIQUE(order_id, product_id)</code> constraint, so a product appears at most once per order.</li>
  <li>Domains are enforced with CHECK constraints, for example status and payment-method lists, positive prices and quantities, and <code>updated_at &ge; created_at</code>.</li>
  <li>Referential actions follow ownership:
    <ul>
      <li>Deleting a customer cascades to their addresses and orders, and from orders on to order lines and payments.</li>
      <li>Deleting a product that has been sold is blocked (<code>ON DELETE RESTRICT</code>), so sales history is kept.</li>
    </ul>
  </li>
</ul>
<h3>4.3 System design: how the database stores and replicates the data</h3>
<ol>
  <li>A client sends SQL to any node over HTTP. A follower forwards the request to the current leader.</li>
  <li>The leader binds the <code>?</code> parameters into literals and appends the statement batch to the Raft log as one entry.</li>
  <li>The entry is replicated over TCP and becomes committed once a majority of nodes has saved it to its BoltDB log.</li>
  <li>Every node applies committed entries in log order through its state machine. The state machine runs the batch in the SQL engine and rolls back the whole batch if any statement fails.</li>
  <li>The SQL engine stores schema and rows through MVCC into a key-value store. That store fsyncs a write-ahead log before each change.</li>
  <li>Reads are served by the leader after it confirms its leadership with a quorum, so every read sees all acknowledged writes (linearizable).</li>
</ol>
{mermaid("system_class.mmd")}
<p class="label">Figure 4.1: System components (Go packages)</p>
{mermaid("write_path_sequence.mmd")}
<p class="label">Figure 4.2: Write path of a replicated transaction</p>

<h2>5. Normalization and Functional Dependencies</h2>
<h3>5.1 Starting point: an unnormalized order sheet</h3>
<p>Without design, a store might keep one wide row per order:</p>
<pre><code>ORDER_SHEET(order_id, order_date, customer_name, customer_email, customer_city,
            {{product_name, category, unit_price, quantity}}*, payment_method, amount)</code></pre>
<p>This sheet has three problems:</p>
<ul>
  <li><strong>Repeating group.</strong> The product list is repeated inside one row, which violates 1NF.</li>
  <li><strong>Redundant data.</strong> Customer and product details are repeated on every order.</li>
  <li><strong>Anomalies.</strong>
    <ul>
      <li>Update: changing a product's price means editing many rows.</li>
      <li>Insertion: a new product cannot be stored until someone orders it.</li>
      <li>Deletion: deleting a customer's only order also deletes the customer.</li>
    </ul>
  </li>
</ul>
<h3>5.2 Functional dependencies</h3>
{table_html(["Relation", "Functional dependencies", "Candidate keys"], [
    ["customers", "customer_id → first_name, last_name, email, created_at, updated_at, status;  email → customer_id", "{customer_id}, {email}"],
    ["addresses", "address_id → customer_id, line1, line2, city, state, postal_code, country, type;  (customer_id, type) → address_id", "{address_id}, {customer_id, type}"],
    ["products", "product_id → product_name, category, unit_price, stock_qty, created_at, updated_at;  product_name → product_id", "{product_id}, {product_name}"],
    ["orders", "order_id → customer_id, order_date, total_amount, status, created_at, updated_at", "{order_id}"],
    ["order_items", "order_item_id → order_id, product_id, quantity, unit_price, line_total, created_at;  (order_id, product_id) → order_item_id", "{order_item_id}, {order_id, product_id}"],
    ["payments", "payment_id → order_id, method, amount, paid_at, status, transaction_ref;  transaction_ref → payment_id", "{payment_id}, {transaction_ref}"],
])}
<h3>5.3 Normal forms</h3>
<ul>
  <li><strong>1NF.</strong> Every attribute is atomic. The repeating product group became separate <code>order_items</code> rows, and addresses became their own relation.</li>
  <li><strong>2NF.</strong> Every relation has a single-attribute primary key, so partial dependencies on part of a composite key cannot occur. In <code>order_items</code>, the alternate key (order_id, product_id) determines the whole row. Product attributes such as name and category are not stored there, because they depend only on product_id. They live in <code>products</code>.</li>
  <li><strong>3NF.</strong> No non-key attribute depends on another non-key attribute. Customer data is not copied into orders, and payment data is not copied into orders.</li>
  <li><strong>BCNF.</strong> In every relation, the determinant of every non-trivial FD (listed above) is a candidate key. So every relation is in BCNF.</li>
</ul>
<h3>5.4 Controlled redundancy</h3>
<p>Three stored values are deliberate redundancies:</p>
<ul>
  <li><strong><code>order_items.unit_price</code></strong> is the historical price at the time of sale. It is not the current <code>products.unit_price</code>, so it is a separate fact and not redundant.</li>
  <li><strong><code>order_items.line_total</code></strong> is derived. It is kept consistent by <code>CHECK (line_total = ROUND(quantity * unit_price, 2))</code>.</li>
  <li><strong><code>orders.total_amount</code></strong> is derived from the order's lines. It is stored for fast reporting, and query Q9 checks it against the lines (it returns 0 rows for the sample data).</li>
</ul>

<h2>6. Advanced SQL Implementation and Query Design</h2>
<h3>6.1 DDL implementation</h3>
<p>The full DDL is in <code>database/schema.sql</code>:</p>
<ul>
  <li>six tables with PRIMARY KEY, FOREIGN KEY (CASCADE and RESTRICT), UNIQUE, NOT NULL, DEFAULT and named CHECK constraints;</li>
  <li>six secondary indexes;</li>
  <li>the <code>order_summary</code> view.</li>
</ul>
<p>The script is portable SQL, tested on SQLite {sqlite3.sqlite_version} and written to run on PostgreSQL.</p>
<p>Sample data is in <code>database/seed.sql</code>: {", ".join(f"{t} {n}" for t, n in counts.items())} rows. It is loaded in one transaction.</p>
<h3>6.2 Queries and their outputs</h3>
<p>The fourteen queries below are documented in <code>database/queries.sql</code>. Their outputs were produced by running them on the seeded database.</p>
{query_section(db)}
<h3>6.3 SQL on the distributed engine</h3>
<p>The Raft-replicated engine supports:</p>
<ul>
  <li><code>CREATE TABLE</code>, <code>INSERT</code>, <code>UPDATE</code>, <code>DELETE</code> and <code>DROP TABLE</code>;</li>
  <li><code>SELECT</code> with <code>WHERE</code> (AND, OR, LIKE and comparisons), a two-table inner equi-<code>JOIN</code>, <code>GROUP BY</code>, <code>HAVING</code>, <code>COUNT/SUM/AVG/MIN/MAX</code>, <code>ORDER BY</code>, <code>LIMIT</code> and <code>OFFSET</code>;</li>
  <li>multi-statement transactions through <code>/transaction</code>.</li>
</ul>
<p>Client values are always sent as bound parameters (<code>?</code> placeholders with a <code>params</code> array). The engine hex-encodes every string literal before parsing, so a value can never be read as SQL. Section 6.4 shows an injection attempt returning 0 rows.</p>
<h3>6.4 Live cluster output</h3>
{md_to_html((results / "cluster_demo.md").read_text(encoding="utf-8"))}

<h2>7. ER Diagram and UML Diagram</h2>
<h3>7.1 ER diagram</h3>
{mermaid("supermarket_er.mmd")}
<p class="label">Figure 7.1: Entity-relationship diagram (PK = primary key, FK = foreign key, UK = unique key)</p>
<h3>7.2 UML class diagram</h3>
{mermaid("supermarket_uml.mmd")}
<p class="label">Figure 7.2: UML class diagram of the supermarket domain (composition shows ownership with cascading delete)</p>

<h2>8. Relational Schema</h2>
<p>Notation: <u>underline</u> = primary key, <em>italic</em> = foreign key.</p>
<pre><code>CUSTOMERS  (<u>customer_id</u>, first_name, last_name, email UNIQUE, created_at, updated_at, status)
ADDRESSES  (<u>address_id</u>, <em>customer_id</em> → CUSTOMERS, line1, line2, city, state, postal_code, country, type)
             UNIQUE (customer_id, type)
PRODUCTS   (<u>product_id</u>, product_name UNIQUE, category, unit_price, stock_qty, created_at, updated_at)
ORDERS     (<u>order_id</u>, <em>customer_id</em> → CUSTOMERS, order_date, total_amount, status, created_at, updated_at)
ORDER_ITEMS(<u>order_item_id</u>, <em>order_id</em> → ORDERS, <em>product_id</em> → PRODUCTS, quantity, unit_price, line_total, created_at)
             UNIQUE (order_id, product_id)
PAYMENTS   (<u>payment_id</u>, <em>order_id</em> → ORDERS, method, amount, paid_at, status, transaction_ref UNIQUE)</code></pre>
{table_html(["Foreign key", "References", "On delete", "Reason"], [
    ["addresses.customer_id", "customers.customer_id", "CASCADE", "Addresses exist only for their customer"],
    ["orders.customer_id", "customers.customer_id", "CASCADE", "Removing a customer removes their order history"],
    ["order_items.order_id", "orders.order_id", "CASCADE", "Lines are part of the order"],
    ["order_items.product_id", "products.product_id", "RESTRICT", "A product that was sold cannot be deleted"],
    ["payments.order_id", "orders.order_id", "CASCADE", "Payments belong to the order"],
])}

<h2>9. Database Tables</h2>
<p>Table structures, attributes, keys and constraints below are read back from the created database. Sample records come from <code>data/sample_db</code>.</p>
{schema_details(db)}

<h2>10. Conclusion and Future Scope</h2>
<h3>10.1 Conclusion</h3>
<p>The project meets its objectives:</p>
<ul>
  <li><strong>Relational design.</strong> The supermarket database is normalized to BCNF and fully constrained. Automated tests check every class of constraint.</li>
  <li><strong>SQL.</strong> Fourteen documented queries cover multi-table joins, correlated, nested and EXISTS subqueries, GROUP BY/HAVING, window functions, a CTE, a view, CASE and UNION.</li>
  <li><strong>Distributed engine.</strong> The same data runs on a database we built ourselves. Every node is an independent process. Writes commit only on a Raft majority, and transactions are all-or-nothing on every replica. Reads are linearizable. Client input is always parameterized.</li>
  <li><strong>Fault tolerance.</strong> In the failover demo (Appendix A), killing the leader caused an automatic election in under 2 seconds. A write sent at that moment still committed, and the restarted node caught up from its own durable log.</li>
</ul>
<h3>10.2 Future scope</h3>
<ul>
  <li>Enforce FOREIGN KEY, UNIQUE and CHECK constraints inside the distributed engine. Today they are enforced in the reference SQL schema.</li>
  <li>Support subqueries and joins of more than two tables in the distributed engine, using a cost-based planner.</li>
  <li>Split data into ranges, each with its own Raft group (multi-Raft sharding), so write throughput scales beyond one leader.</li>
  <li>Allow nodes to join and leave a running cluster.</li>
  <li>Add TLS between nodes and clients, plus authentication and role-based authorization.</li>
  <li>Store data in an on-disk LSM storage engine with typed columns and secondary indexes.</li>
</ul>

<h2>Appendix A: Testing and Validation</h2>
{table_html(["Suite", "Command", "What it proves"], [
    ["Go unit and integration tests", "go test ./...", "MVCC snapshot isolation and conflicts; SQL CRUD, filters, grouping, joins; parameter binding blocks injection; the WAL stays correct after rollback; a 3-node Raft cluster over TCP replicates, fails over and recovers from its log"],
    ["Schema tests (16)", "python -m unittest discover -s database -v", "Seed counts; no dangling FKs; PK, UNIQUE, FK, RESTRICT and 8 CHECK rules reject bad rows; ON DELETE CASCADE removes dependents; every documented query runs and returns the expected rows"],
    ["Cluster smoke test", "scripts/smoke.ps1 or scripts/smoke.sh", "Waits for a leader, then runs DDL, DML, a transaction and a read through the HTTP API"],
    ["Cluster seed and demo", "python database/seed_cluster.py", "Loads all six tables through parameterized transactions and records the query outputs"],
])}
<h3>A.1 Failover demonstration</h3>
{md_to_html((results / "failover_demo.md").read_text(encoding="utf-8"))}
<h3>A.2 Screenshots</h3>
{image("ui-join-query.png", "Figure A.1: React console running a join on the live cluster")}
{image("ui-group-by.png", "Figure A.2: GROUP BY with aggregates on the live cluster")}

<h2>Appendix B: Setup and Demonstration</h2>
<ol>
  <li>Install Go 1.26 or newer, Python 3.10 or newer and Node.js 18 or newer. Docker Desktop is optional.</li>
  <li>Clone the repository: <code>git clone {e(team["github"])}</code></li>
  <li>Run the tests: <code>go test ./...</code> and <code>python -m unittest discover -s database -v</code></li>
  <li>Build the reference database and record query outputs: <code>python database/run_queries.py --db database/supermarket.db</code></li>
  <li>Start a three-node cluster with <code>scripts/cluster.ps1</code> (Windows), <code>scripts/cluster.sh</code> (Linux/macOS) or <code>docker compose up -d --build</code>.</li>
  <li>Create and populate the tables on the cluster: <code>python database/seed_cluster.py</code></li>
  <li>Start the UI: <code>cd frontend &amp;&amp; npm ci &amp;&amp; npm run dev</code>, then open <code>http://localhost:5173</code></li>
  <li>Demonstrate failover. Find the leader with <code>GET /health</code>, stop that process or container, then send a write to another node. It still commits. Restart the stopped node and confirm it catches up.</li>
</ol>

<script src="https://cdn.jsdelivr.net/npm/mermaid@10.9.1/dist/mermaid.min.js"></script>
<script>mermaid.initialize({{ startOnLoad: true, theme: "neutral", er: {{ useMaxWidth: true }}, flowchart: {{ useMaxWidth: true }} }});</script>
</body></html>"""
    out_html = REPORT / "report.html"
    out_html.write_text(page, encoding="utf-8")
    print(f"wrote {out_html}")

    browser = next((p for p in [
        shutil.which("chrome"), shutil.which("msedge"), shutil.which("google-chrome"), shutil.which("chromium"),
        r"C:\Program Files\Google\Chrome\Application\chrome.exe",
        r"C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe",
    ] if p and os.path.exists(p)), None)
    if not browser:
        print("Chrome/Edge not found: open report.html in a browser and print it to PDF.")
        return
    pdf = REPORT / "Project_Report.pdf"
    subprocess.run([browser, "--headless=new", "--disable-gpu", "--no-pdf-header-footer", "--virtual-time-budget=20000",
                    f"--print-to-pdf={pdf}", out_html.as_uri()], check=True, capture_output=True)
    print(f"wrote {pdf}")


if __name__ == "__main__":
    main()
