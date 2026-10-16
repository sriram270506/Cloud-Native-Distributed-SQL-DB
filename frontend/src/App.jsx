import React, {useEffect, useMemo, useState} from 'react'

const API = 'http://localhost:8080'
const SAMPLE_FILES = ['customers', 'addresses', 'products', 'orders', 'order_items', 'payments']

function parseCSV(text) {
  const lines = text.trim().split(/\r?\n/)
  const headers = lines.shift().split(',').map(value => value.trim())
  return lines.filter(Boolean).map(line => {
    const values = line.split(',')
    return headers.reduce((row, header, index) => ({...row, [header]: values[index] ?? ''}), {})
  })
}

// A ?sql= link opens the console with that query and runs it.
const INITIAL_SQL = new URLSearchParams(window.location.search).get('sql')

export default function App() {
  const [sql, setSql] = useState(INITIAL_SQL || 'SELECT category, COUNT(*) AS products, SUM(stock_qty) AS units FROM products GROUP BY category ORDER BY units DESC')
  const [result, setResult] = useState(null)
  const [tables, setTables] = useState([])
  const [status, setStatus] = useState('Checking connection')
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const tableCount = useMemo(() => tables.length, [tables])

  async function refreshTables() {
    const response = await fetch(`${API}/tables`)
    if (!response.ok) throw new Error('Database API is not available')
    const data = await response.json()
    setTables(data.tables || [])
    setStatus('Connected')
  }

  useEffect(() => {
    refreshTables().then(() => INITIAL_SQL && executeQuery()).catch(() => setStatus('Backend offline'))
  }, [])

  async function executeQuery() {
    setBusy(true)
    setError('')
    setMessage('')
    try {
      const response = await fetch(`${API}/query`, {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({sql})
      })
      const text = await response.text()
      if (!response.ok) throw new Error(text || 'Query failed')
      const data = text ? JSON.parse(text) : {}
      if (data.columns) setResult(data)
      else setResult(null)
      setMessage(data.message || 'Query completed')
      await refreshTables()
    } catch (err) {
      setError(err.message)
    } finally {
      setBusy(false)
    }
  }

  async function loadSampleData() {
    setBusy(true)
    setError('')
    setMessage('Loading supermarket data...')
    try {
      for (const table of SAMPLE_FILES) {
        if (tables.includes(table)) continue
        const response = await fetch(`/data/sample_db/${table}.csv`)
        if (!response.ok) continue
        const rows = parseCSV(await response.text())
        if (!rows.length) continue
        const columns = Object.keys(rows[0])
        // Row values travel as bound parameters, never inside the SQL text.
        const insert = `INSERT INTO ${table} (${columns.join(', ')}) VALUES (${columns.map(() => '?').join(', ')})`
        const statements = [`CREATE TABLE ${table} (${columns.map(column => `${column} TEXT`).join(', ')})`, ...rows.map(() => insert)]
        const statement_params = [[], ...rows.map(row => columns.map(column => row[column] ?? ''))]
        const transaction = await fetch(`${API}/transaction`, {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({statements, statement_params})
        })
        if (!transaction.ok && !/already|exists/i.test(await transaction.text())) {
          throw new Error(`Could not load ${table}`)
        }
      }
      await refreshTables()
      setMessage('Supermarket data is ready')
    } catch (err) {
      setError(err.message)
    } finally {
      setBusy(false)
    }
  }

  function chooseQuery(value) {
    setSql(value)
    setResult(null)
    setMessage('')
  }

  return (
    <div className="shell">
      <header className="topbar">
        <div className="brand">
          <div className="brand-mark">M</div>
          <div>
            <strong>Market data</strong>
            <span>Operations console</span>
          </div>
        </div>
        <div className="connection"><i className={status === 'Connected' ? 'online' : ''} />{status}</div>
      </header>

      <main className="workspace">
        <section className="page-heading">
          <div>
            <p className="eyebrow">Store analytics</p>
            <h1>Database workspace</h1>
            <p className="subheading">Manage stock, customers and orders from one place.</p>
          </div>
          <button className="secondary" onClick={loadSampleData} disabled={busy}>Load store data</button>
        </section>

        <section className="stats">
          <div className="stat"><span>Connection</span><strong>{status === 'Connected' ? 'Healthy' : 'Offline'}</strong><small>Raft database API</small></div>
          <div className="stat"><span>Tables</span><strong>{tableCount}</strong><small>Available in this store</small></div>
          <div className="stat"><span>Query mode</span><strong>Live</strong><small>Changes are persisted</small></div>
        </section>

        <section className="content-grid">
          <aside className="panel tables-panel">
            <div className="panel-title"><h2>Store tables</h2><span>{tableCount}</span></div>
            {tables.length === 0 ? <p className="muted">No tables yet. Load store data to get started.</p> :
              <ul className="table-list">{tables.map(table => <li key={table}><span className="table-icon">▦</span>{table}</li>)}</ul>}
          </aside>

          <section className="panel query-panel">
            <div className="panel-title"><div><h2>SQL query</h2><p>Run reads and changes against the live store.</p></div></div>
            <textarea value={sql} onChange={event => setSql(event.target.value)} spellCheck="false" />
            <div className="query-actions">
              <button className="primary" onClick={executeQuery} disabled={busy}>{busy ? 'Running...' : 'Run query'}</button>
              <button className="text-button" onClick={() => setSql('')}>Clear</button>
            </div>
            <div className="query-shortcuts">
              <button onClick={() => chooseQuery('SELECT * FROM products LIMIT 25')}>Product stock</button>
              <button onClick={() => chooseQuery('SELECT status, COUNT(*) AS orders, SUM(total_amount) AS revenue FROM orders GROUP BY status ORDER BY revenue DESC')}>Order summary</button>
              <button onClick={() => chooseQuery('SELECT * FROM products WHERE stock_qty < 50 ORDER BY stock_qty ASC')}>Low stock</button>
            </div>
            {message && <div className="notice">{message}</div>}
            {error && <div className="error">{error}</div>}
          </section>
        </section>

        <section className="panel results-panel">
          <div className="panel-title"><div><h2>Results</h2><p>{result ? `${result.rows.length} rows returned` : 'Run a query to see results here.'}</p></div></div>
          {result && <div className="result-scroll"><table><thead><tr>{result.columns.map(column => <th key={column}>{column}</th>)}</tr></thead><tbody>
            {result.rows.map((row, index) => <tr key={index}>{result.columns.map(column => <td key={column}>{row[column] ?? ''}</td>)}</tr>)}
          </tbody></table></div>}
        </section>
      </main>
    </div>
  )
}
