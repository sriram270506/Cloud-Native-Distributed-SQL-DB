import React, {useEffect, useState} from 'react'
import initSqlJs from 'sql.js'

const TABLE_FILES = {
  customers: 'customers.csv',
  addresses: 'addresses.csv',
  products: 'products.csv',
  orders: 'orders.csv',
  order_items: 'order_items.csv',
  payments: 'payments.csv'
}

function parseCSV(text){
  const lines = text.trim().split('\n')
  const hdr = lines[0].split(',').map(h=>h.trim())
  return lines.slice(1).map(l=>{
    // simple split, acceptable for sample data
    const cols = l.split(',')
    const obj = {}
    hdr.forEach((h,i)=> obj[h]=cols[i]===undefined?null:cols[i])
    return obj
  })
}

function escapeSql(v){
  if(v===null||v===undefined) return 'NULL'
  return "'"+String(v).replace(/'/g,"''")+"'"
}

export default function App(){
  const [db, setDb] = useState(null)
  const [sql, setSql] = useState('SELECT * FROM customers LIMIT 50')
  const [result, setResult] = useState(null)
  const [err, setErr] = useState(null)
  const [status, setStatus] = useState('initializing')
  const [tables, setTables] = useState([])

  useEffect(()=>{
    let mounted = true
    ;(async()=>{
      try{
        const SQL = await initSqlJs({locateFile:file=>`/node_modules/sql.js/dist/${file}`})
        const db = new SQL.Database()
        // load CSVs and create tables
        for(const [tbl,file] of Object.entries(TABLE_FILES)){
          setStatus(`loading ${file}`)
          const resp = await fetch(`/data/sample_db/${file}`)
          if(!resp.ok){ console.warn('missing', file); continue }
          const txt = await resp.text()
          const rows = parseCSV(txt).filter(r=> Object.keys(r).length>0)
          if(rows.length===0) continue
          const cols = Object.keys(rows[0])
          const createCols = cols.map(c=>`\"${c}\" TEXT`).join(',')
          // create table without quoting table name to avoid case-sensitive identifiers
          db.run(`CREATE TABLE IF NOT EXISTS ${tbl} (${createCols});`)
          const insertStmt = db.prepare(`INSERT INTO ${tbl} VALUES (${cols.map(_=>'?').join(',')})`)
          db.run('BEGIN')
          for(const r of rows){
            const vals = cols.map(c=> r[c])
            insertStmt.run(vals)
          }
          db.run('COMMIT')
          insertStmt.free()
        }
        // collect table list
        const tbls = db.exec("SELECT name FROM sqlite_master WHERE type='table'")
        if(tbls && tbls[0] && tbls[0].values){
          setTables(tbls[0].values.map(v=>v[0]))
        }
        if(mounted) { setDb(db); setStatus('ready') }
      }catch(e){ console.error(e); setErr('Failed to initialize DB'); setStatus('error') }
    })()
    return ()=>{ mounted=false }
  },[])

  function run(){
    setErr(null)
    setResult(null)
    if(!db){ setErr('DB not ready'); return }
    try{
      const res = db.exec(sql)
      if(res.length===0){ setResult({columns:[], values:[]}); return }
      // use first result set
      setResult({columns: res[0].columns, values: res[0].values})
    }catch(e){ setErr(String(e)) }
  }

  return (
    <div className="app card">
      <header className="header">
        <h2>SQL Playground</h2>
        <div className="status">Status: <strong>{status}</strong></div>
      </header>

      <div className="sidebar">
        <div className="section">
          <h4>Tables</h4>
          <ul>
            {tables.map(t=> <li key={t}>{t}</li>)}
          </ul>
        </div>
        <div className="section">
          <h4>Examples</h4>
          <select onChange={e=>setSql(e.target.value)}>
            <option>SELECT * FROM customers LIMIT 50</option>
            <option>SELECT * FROM products LIMIT 50</option>
            <option>SELECT order_id, total_amount FROM orders</option>
            <option>SELECT p.product_name, oi.quantity FROM order_items oi JOIN products p ON oi.product_id=p.product_id LIMIT 50</option>
          </select>
        </div>
      </div>

      <main>
        <textarea className="editor" value={sql} onChange={e=>setSql(e.target.value)} />
        <div className="controls">
          <button onClick={run}>Run</button>
        </div>

        {err && <div className="err">Error: {err}</div>}

        <div className="table">
          {result && (
            <table>
              <thead>
                <tr>{result.columns.map(c=> <th key={c}>{c}</th>)}</tr>
              </thead>
              <tbody>
                {result.values.map((r,ri)=> <tr key={ri}>{r.map((v,ci)=><td key={ci}>{v}</td>)}</tr>)}
              </tbody>
            </table>
          )}
        </div>
      </main>
    </div>
  )
}
