"""Build the project report (HTML + PDF) from the repository's own artifacts.

    python docs/report/build_report.py

Reads docs/report/team.json, database/*.sql, data/sample_db/*.csv, docs/*.mmd and
docs/screenshots/*.png, writes docs/report/report.html, then prints it to
docs/report/Project_Report.pdf with headless Chrome or Edge (needs internet for the diagrams).
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
SHOWCASE = ["Q2", "Q4", "Q5", "Q8"]  # queries printed with their output
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


def table_details(db):
    """Per-table structure (read back from SQLite) plus three sample rows."""
    out = []
    for number, table in enumerate(TABLES, 1):
        fks = {row[3]: f"FK → {row[2]}" for row in db.execute(f"PRAGMA foreign_key_list({table})")}
        uniques = set()
        for index in db.execute(f"PRAGMA index_list({table})"):
            cols = [c[2] for c in db.execute(f"PRAGMA index_info({index[1]})")]
            if index[2] and len(cols) == 1:
                uniques.add(cols[0])
        rows = []
        for _, name, ctype, notnull, _, pk in db.execute(f"PRAGMA table_info({table})"):
            keys = ["PK"] if pk else []
            keys += [fks[name]] if name in fks else []
            keys += ["UNIQUE"] if name in uniques else []
            rows.append([name, ctype, "yes" if notnull or pk else "no", ", ".join(keys)])
        with open(ROOT / "data" / "sample_db" / f"{table}.csv", newline="") as handle:
            sample = list(csv.reader(handle))
        out.append(f"""<div class="keep"><h3>9.{number} {table}</h3>
{table_html(["Column", "Type", "Required", "Key"], rows, "small")}
<p class="label">Sample records (3 of {len(sample) - 1})</p>
{table_html(sample[0], sample[1:4], "small")}</div>""")
    return "\n".join(out)


def showcase(db):
    queries = {qid: (title, purpose, sql) for qid, title, purpose, sql in parse_queries((ROOT / "database" / "queries.sql").read_text())}
    listing = table_html(["#", "What it shows"], [[qid, title] for qid, (title, _, _) in queries.items()], "small")
    parts = []
    for qid in SHOWCASE:
        title, purpose, sql = queries[qid]
        cursor = db.execute(sql)
        columns = [d[0] for d in cursor.description]
        rows = cursor.fetchall()
        parts.append(f"""<div class="keep"><h4>{qid}. {e(title)}</h4><p>{e(purpose)}</p>
<pre><code>{e(sql)}</code></pre>{table_html(columns, rows[:6], "small")}</div>""")
    return listing, "\n".join(parts)


def main():
    team = json.loads((REPORT / "team.json").read_text())
    db = sqlite3.connect(":memory:")
    build(db)
    members = table_html(["#", "Name", "Roll number"], [[i + 1, m["name"], m["roll_no"]] for i, m in enumerate(team["members"])])
    listing, examples = showcase(db)

    page = f"""<!doctype html><html><head><meta charset="utf-8"><title>Project Report</title>
<style>
@page {{ size: A4; margin: 16mm 15mm; }}
body {{ font-family: "Segoe UI", Arial, sans-serif; font-size: 10.5pt; line-height: 1.45; color: #1d2329; }}
h1 {{ font-size: 22pt; margin: 0 0 8px; color: #174f3a; }}
h2 {{ font-size: 15pt; border-bottom: 2px solid #1f6f50; padding-bottom: 3px; margin: 22px 0 8px; color: #174f3a; }}
h2.page {{ page-break-before: always; margin-top: 0; }}
h3 {{ font-size: 12pt; margin: 14px 0 5px; color: #174f3a; }}
h4 {{ font-size: 10.5pt; margin: 10px 0 3px; }}
p {{ margin: 4px 0 6px; }}
table {{ border-collapse: collapse; width: 100%; margin: 5px 0 8px; font-size: 9pt; }}
th, td {{ border: 1px solid #c9d1d6; padding: 3px 6px; text-align: left; vertical-align: top; }}
th {{ background: #e8f1ec; }}
table.small {{ font-size: 8pt; }}
tr, .keep {{ page-break-inside: avoid; }}
pre {{ background: #f5f7f8; border: 1px solid #dde3e6; padding: 5px 8px; font-size: 8pt; white-space: pre-wrap; margin: 4px 0; }}
pre.mermaid svg {{ max-height: 105mm; }}
pre.mermaid {{ background: none; border: none; text-align: center; page-break-inside: avoid; }}
.label {{ font-weight: 600; margin: 6px 0 2px; font-size: 9pt; }}
.box {{ background: #eef5f1; border: 1px solid #cfe2d8; padding: 8px 12px; margin: 8px 0; }}
figure {{ margin: 8px 0; text-align: center; page-break-inside: avoid; }}
figure img {{ max-width: 88%; border: 1px solid #c9d1d6; }}
figcaption {{ font-size: 9pt; color: #555; }}
.title {{ text-align: center; padding-top: 90px; }}
.title .course {{ font-size: 13pt; color: #444; }}
.title .meta {{ font-size: 11.5pt; margin: 6px 0; }}
.title table {{ width: 75%; margin: 18px auto; font-size: 10.5pt; }}
</style></head><body>

<section class="title">
  <p class="course">{e(team["course"])} &mdash; Project Report</p>
  <h2 style="border:none; font-size:13pt; color:#444">1. Project Title</h2>
  <h1>{e(team["project_name"])}</h1>
  <p class="meta">Batch {e(team["batch"])} &middot; Group {e(team["group"])}</p>
  <h2 style="border:none; font-size:13pt; color:#444; margin-top:40px">2. Team Members</h2>
  {members}
  <p class="meta" style="margin-top:30px">GitHub repository:<br><a href="{e(team["github"])}">{e(team["github"])}</a></p>
</section>

<h2 class="page">3. Introduction and Problem Statement</h2>
<h3>3.1 The problem</h3>
<p>An online store has to keep track of its customers, their addresses, the products it sells, the orders people place and the payments for those orders. Most college projects keep all of this in one database on one computer, which causes three problems:</p>
<ul>
  <li><strong>One crash stops the shop.</strong> If that single computer fails, nobody can buy anything, and orders that were being saved may be lost.</li>
  <li><strong>It cannot grow.</strong> When more shoppers arrive, the only option is to buy a bigger, more expensive computer.</li>
  <li><strong>Simultaneous buyers can clash.</strong> If two people buy at the same moment, one of them might see half-saved information.</li>
</ul>
<h3>3.2 Our solution: a NewSQL database</h3>
<p><strong>NewSQL</strong> means a database that looks and feels like a normal SQL database (tables, keys, <code>SELECT</code> queries, transactions) but runs on several computers at once. Well-known examples are Google Spanner and CockroachDB. We built a small one ourselves in the Go language and used it to run an e-commerce database. It relies on three main ideas.</p>
{table_html(["Idea", "In simple words", "What it gives the store"], [
    ["Horizontal scaling", "Grow by adding more computers (nodes) instead of buying one bigger computer.", "Our cluster has 3 nodes, and each holds a full copy of the data."],
    ["Raft consensus", "The nodes vote to choose a leader. The leader copies every change to the others, and a change counts only when most nodes (2 of 3) have saved it.", "If a node crashes, no saved order is lost. If the leader crashes, a new leader is chosen automatically in under 2 seconds."],
    ["MVCC transactions", "Multi-Version Concurrency Control: each change makes a new version of a row instead of overwriting it, so readers see a clean snapshot.", "Shoppers never see half-saved orders. A group of statements either all succeed or all fail."],
])}
<h3>3.3 Objectives</h3>
<ul>
  <li>Design a clean (normalized) e-commerce database with proper keys and rules.</li>
  <li>Make it run on 3 computers with Raft, so it survives a crash.</li>
  <li>Support SQL: create, insert, update, delete, search, join, group and summarise.</li>
  <li>Prove that everything works with automated tests and a live crash test.</li>
</ul>

<h2>4. Conceptual and Logical Database Design</h2>
<h3>4.1 Conceptual design: what the store needs to remember</h3>
{table_html(["Entity (thing)", "What it is", "Unique ID"], [
    ["Customer", "A person with an account", "customer_id"],
    ["Address", "A home or work address of a customer", "address_id"],
    ["Product", "Something we sell, with its price and stock", "product_id"],
    ["Order", "One purchase by one customer on one date", "order_id"],
    ["Order Item", "One product line inside an order (for example, 2 × Widget A)", "order_item_id"],
    ["Payment", "Money paid for an order", "payment_id"],
])}
{table_html(["Relationship", "Type", "Meaning"], [
    ["Customer has Addresses", "One-to-many", "One customer can have many addresses."],
    ["Customer places Orders", "One-to-many", "One customer can place many orders; every order has exactly one customer."],
    ["Order contains Products", "Many-to-many", "An order has many products and a product appears in many orders. The order_items table sits in between."],
    ["Order receives Payments", "One-to-many", "An order can be paid in one or more payments."],
])}
<h3>4.2 Logical design: turning the entities into tables</h3>
<ul>
  <li>Every entity becomes a table, and its unique ID becomes the <strong>primary key</strong>.</li>
  <li>Every one-to-many relationship becomes a <strong>foreign key</strong> on the "many" side. For example, <code>orders.customer_id</code> points to <code>customers</code>.</li>
  <li>The many-to-many link between orders and products becomes its own table, <code>order_items</code>. It allows each product only once per order.</li>
  <li><strong>CHECK</strong> rules stop bad data, such as negative prices, a quantity of 0 or an unknown order status.</li>
  <li><strong>ON DELETE CASCADE</strong>: deleting a customer also deletes their addresses, orders, order items and payments. A product that has been sold cannot be deleted, so sales history stays correct.</li>
</ul>
<h3>4.3 How one order is saved across the computers</h3>
{mermaid("write_path_sequence.mmd")}

<h2 class="page">5. Normalization and Functional Dependencies</h2>
<p><strong>Normalization</strong> means splitting data into tables so that every fact is stored in exactly one place. To see why this matters, imagine the store kept everything in one big sheet:</p>
<pre><code>order_id | date       | customer | email           | products ordered                 | paid
1        | 2024-06-01 | Emma     | emma@example.com | Widget A x1 (19.99), Accessory 1 x2 | card</code></pre>
<p>This sheet causes three kinds of trouble:</p>
<ul>
  <li><strong>Update problem.</strong> If Emma changes her email, every one of her orders must be edited. Missing one leaves the data disagreeing with itself.</li>
  <li><strong>Insert problem.</strong> A new product cannot be added until somebody orders it.</li>
  <li><strong>Delete problem.</strong> Deleting Emma's only order also deletes everything we knew about Emma.</li>
</ul>
<h3>5.1 How our tables meet each normal form</h3>
{table_html(["Normal form", "The rule in plain words", "How our e-commerce database follows it"], [
    ["1NF (First Normal Form)", "Every cell holds one value. No lists inside a cell. Every row can be identified.", "The 'products ordered' list became separate rows in order_items: an order with 3 products has 3 rows. Every table has a primary key."],
    ["2NF (Second Normal Form)", "Already in 1NF, and every column depends on the whole key, not just part of it.", "Product name, category and price depend only on product_id, so they live in products. order_items keeps just product_id. Every table has a one-column key, so a 'half-key' dependency cannot happen."],
    ["3NF (Third Normal Form)", "Already in 2NF, and no column depends on another non-key column.", "orders stores only customer_id, never the customer's email or city. Those depend on the customer, not the order, so they live in customers and addresses. Payment details live in payments."],
])}
<div class="box"><strong>Result:</strong> every fact (an email, a price, an address) is stored exactly once. Changing it in one place updates it everywhere. Our tables actually meet BCNF, which is slightly stricter than 3NF: in every table, only a key decides the other columns.</div>
<h3>5.2 Functional dependencies</h3>
<p>A functional dependency <strong>X → Y</strong> means: if you know X, there is exactly one possible Y. For example, knowing <code>product_id = 1</code> tells you its name is "Widget A" and its price is 19.99. In our tables, X is always a key, which is what makes them normalized.</p>
{table_html(["Table", "If you know… (X)", "…you know exactly (Y)"], [
    ["customers", "customer_id, or email", "first_name, last_name, email, created_at, updated_at, status"],
    ["addresses", "address_id, or (customer_id + type)", "customer_id, line1, line2, city, state, postal_code, country, type"],
    ["products", "product_id, or product_name", "product_name, category, unit_price, stock_qty, created_at, updated_at"],
    ["orders", "order_id", "customer_id, order_date, total_amount, status, created_at, updated_at"],
    ["order_items", "order_item_id, or (order_id + product_id)", "order_id, product_id, quantity, unit_price, line_total, created_at"],
    ["payments", "payment_id, or transaction_ref", "order_id, method, amount, paid_at, status, transaction_ref"],
])}
<p>Some stored values are copies that could be worked out from other data. We keep them on purpose and guard them:</p>
<ul>
  <li><code>line_total</code> is checked by a CHECK rule to equal quantity × price.</li>
  <li><code>order_items.unit_price</code> is the price paid on the day of the sale, which can differ from today's catalogue price.</li>
  <li><code>total_amount</code> on each order is checked against the sum of its lines by query Q9.</li>
</ul>

<h2>6. Advanced SQL Implementation and Query Design</h2>
<h3>6.1 Building the database (DDL)</h3>
<p>The tables are created by <code>database/schema.sql</code>, which contains:</p>
<ul>
  <li>6 tables, each with a <strong>PRIMARY KEY</strong>;</li>
  <li><strong>FOREIGN KEY</strong> links with <strong>ON DELETE CASCADE</strong>;</li>
  <li><strong>UNIQUE</strong> rules (email, product name, transaction reference);</li>
  <li><strong>CHECK</strong> rules (prices &gt; 0, valid status values, line_total = quantity × price);</li>
  <li>indexes for speed and one <strong>view</strong>.</li>
</ul>
<p><code>database/seed.sql</code> fills the tables with sample data: 10 customers, 10 addresses, 10 products, 10 orders, 12 order items and 10 payments.</p>
<h3>6.2 The 14 documented queries</h3>
<p>All queries are in <code>database/queries.sql</code>, each with a comment saying what it does. They cover multi-table joins, subqueries, GROUP BY with aggregates, HAVING, a window function, a view, CASE and UNION.</p>
{listing}
<h3>6.3 Four example queries with their real output</h3>
{examples}
<h3>6.4 Transactions and safe queries on our distributed database</h3>
<ul>
  <li><strong>Transactions.</strong> Several statements (for example: create the order, add its items, record the payment) are sent together. Raft copies them as one unit, and every node runs them. If any statement fails, all of them are undone on every node, thanks to MVCC.</li>
  <li><strong>Parameterized queries.</strong> The user's input is sent separately from the SQL text, as <code>WHERE email = ?</code> plus the value. A hacking attempt like <code>x' OR '1'='1</code> is treated as a plain piece of text and matches 0 rows. Our tests check this.</li>
</ul>

<h2 class="page">7. ER Diagram and UML Diagram</h2>
<h3>7.1 ER diagram</h3>
<p>Boxes are tables. PK marks the primary key, FK a foreign key and UK a unique column. The line ends show how many: one, or many.</p>
{mermaid("ecommerce_er.mmd")}
<h3>7.2 UML class diagram</h3>
{mermaid("ecommerce_uml.mmd")}

<h2>8. Relational Schema</h2>
<p>Notation: <u>underlined</u> = primary key, <em>italic</em> = foreign key (with the table it points to).</p>
<pre><code>CUSTOMERS   (<u>customer_id</u>, first_name, last_name, email UNIQUE, created_at, updated_at, status)
ADDRESSES   (<u>address_id</u>, <em>customer_id</em> → CUSTOMERS, line1, line2, city, state, postal_code, country, type)
PRODUCTS    (<u>product_id</u>, product_name UNIQUE, category, unit_price, stock_qty, created_at, updated_at)
ORDERS      (<u>order_id</u>, <em>customer_id</em> → CUSTOMERS, order_date, total_amount, status, created_at, updated_at)
ORDER_ITEMS (<u>order_item_id</u>, <em>order_id</em> → ORDERS, <em>product_id</em> → PRODUCTS, quantity, unit_price, line_total, created_at)
PAYMENTS    (<u>payment_id</u>, <em>order_id</em> → ORDERS, method, amount, paid_at, status, transaction_ref UNIQUE)</code></pre>
{table_html(["Foreign key", "Points to", "When the parent row is deleted"], [
    ["addresses.customer_id", "customers", "Addresses are deleted too (CASCADE)"],
    ["orders.customer_id", "customers", "Orders are deleted too (CASCADE)"],
    ["order_items.order_id", "orders", "Order lines are deleted too (CASCADE)"],
    ["order_items.product_id", "products", "Blocked: a product that was sold cannot be deleted (RESTRICT)"],
    ["payments.order_id", "orders", "Payments are deleted too (CASCADE)"],
])}

<h2>9. Database Tables</h2>
<p>The structure below is read back directly from the created database. All sample records are in <code>data/sample_db/</code>.</p>
{table_details(db)}

<h2>10. Conclusion and Future Scope</h2>
<h3>10.1 Conclusion</h3>
<ul>
  <li>We built a working <strong>NewSQL database</strong>: normal SQL tables, copied across 3 computers.</li>
  <li><strong>Raft</strong> makes sure a change is saved only when most computers agree. In our crash test we killed the leader, a new leader took over in under 2 seconds, and no order was lost.</li>
  <li><strong>MVCC transactions</strong> let shoppers read clean, consistent data while others buy, and make a group of changes all-or-nothing.</li>
  <li>The e-commerce schema is <strong>normalized (3NF, in fact BCNF)</strong> and protected with primary keys, foreign keys, UNIQUE and CHECK rules.</li>
  <li>Everything is checked automatically by Go tests, 16 SQL tests and a live failover demo (see the appendix).</li>
</ul>
<h3>10.2 Future scope</h3>
<ul>
  <li>Enforce foreign keys, UNIQUE and CHECK rules inside our own distributed engine, and support subqueries there too.</li>
  <li>Split large tables across different groups of computers (sharding), so writes also scale with more machines.</li>
  <li>Add logins, passwords and encrypted connections.</li>
</ul>

<h2>Appendix: Testing and Demonstration</h2>
{table_html(["Test", "Command", "What it proves"], [
    ["Engine tests (Go)", "go test ./...", "MVCC isolation; SQL commands; SQL-injection safety; 3 computers copy data, survive a leader crash and recover"],
    ["Database tests (16)", "python -m unittest discover -s database -v", "Keys, foreign keys, UNIQUE and CHECK rules reject bad data; CASCADE deletes work; every query gives the right answer"],
    ["Live demo", "scripts/cluster.ps1, then python database/seed_cluster.py", "Starts 3 computers, loads the e-commerce data and runs queries on them"],
])}
{image("ui-join-query.png", "Web console running a JOIN on the live 3-node cluster")}

<script src="https://cdn.jsdelivr.net/npm/mermaid@10.9.1/dist/mermaid.min.js"></script>
<script>mermaid.initialize({{ startOnLoad: true, theme: "neutral" }});</script>
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
