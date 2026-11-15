import { useEffect, useState } from 'react'
import { api } from '../api'

const ICON = { product: '📦', project: '🗂️', feature: '✨' }

export default function TypePicker({ onOpen }) {
  const [types, setTypes] = useState([])
  const [sessions, setSessions] = useState([])
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    api.prdTypes().then(setTypes).catch((e) => setErr(e.message))
    api.sessions().then(setSessions).catch(() => {})
  }, [])

  async function start(t) {
    setBusy(true); setErr('')
    try { const st = await api.createSession(t); onOpen(st.session.id) }
    catch (e) { setErr(e.message); setBusy(false) }
  }

  return (
    <main className="picker">
      <h1>What would you like to write a PRD for?</h1>
      <p className="muted">The questions and section order adapt to what you pick.</p>
      {err && <div className="note err">{err}</div>}
      <div className="type-grid">
        {types.map((t) => (
          <button key={t.type} className="type-card" disabled={busy} onClick={() => start(t.type)}>
            <div className="type-icon">{ICON[t.type]}</div>
            <h3>{t.title}</h3>
            <p>{t.description}</p>
            <ol className="sections">{t.sections.map((s) => <li key={s}>{s}</li>)}</ol>
          </button>
        ))}
      </div>

      {sessions.length > 0 && (
        <>
          <h2>Continue a previous PRD</h2>
          <ul className="session-list">
            {sessions.map((s) => (
              <li key={s.id}>
                <button className="session-row" onClick={() => onOpen(s.id)}>
                  <span>{ICON[s.prd_type]} <b>{s.title || 'Untitled'}</b> <span className="muted">· {s.prd_type}</span></span>
                  <span className="muted">{new Date(s.updated_at).toLocaleString()}</span>
                </button>
              </li>
            ))}
          </ul>
        </>
      )}
    </main>
  )
}
