import { useEffect, useState } from 'react'
import { api } from './api'
import LLMGate from './components/LLMGate.jsx'
import TypePicker from './components/TypePicker.jsx'
import Workspace from './components/Workspace.jsx'

export default function App() {
  const [llm, setLlm] = useState(null) // /llm/status
  const [sessionId, setSessionId] = useState(null)
  const [backendDown, setBackendDown] = useState(false)

  const refresh = () =>
    api.llmStatus().then((s) => { setLlm(s); setBackendDown(false) }).catch(() => setBackendDown(true))
  useEffect(() => { refresh() }, [])

  if (backendDown)
    return (
      <div className="center-screen">
        <div className="card narrow">
          <h2>Can't reach the backend</h2>
          <p className="muted">Make sure the Go API is running on port 8080 (or run <code>docker compose up</code>).</p>
          <button className="btn primary" onClick={refresh}>Retry</button>
        </div>
      </div>
    )
  if (!llm) return <div className="center-screen muted">Loading…</div>

  // The gate always comes first: no working LLM, no PRD.
  if (!llm.ready) return <LLMGate defaultLocalUrl={llm.default_local_url} onReady={refresh} />

  return (
    <div className="app">
      <header className="topbar">
        <div className="brand"><span className="logo">PRD</span> Assistant</div>
        <div className="topbar-right">
          <span className="badge ok" title="The API key is kept in server memory only">
            {llm.info.mode === 'local' ? 'Local LLM' : llm.info.provider} · {llm.info.model}
          </span>
          {sessionId && <button className="btn" onClick={() => setSessionId(null)}>All PRDs</button>}
          <button className="btn" onClick={() => api.llmReset().then(() => { setSessionId(null); refresh() })}>Change LLM</button>
        </div>
      </header>
      {sessionId
        ? <Workspace key={sessionId} sessionId={sessionId} />
        : <TypePicker onOpen={setSessionId} />}
    </div>
  )
}
