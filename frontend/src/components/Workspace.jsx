import { useCallback, useEffect, useRef, useState } from 'react'
import ReactMarkdown from 'react-markdown'
import { api } from '../api'
import { useDictation } from '../useDictation'
import TagPicker from './TagPicker.jsx'
import Preview from './Preview.jsx'

export default function Workspace({ sessionId }) {
  const [st, setSt] = useState(null)
  const [preview, setPreview] = useState(null)
  const [filter, setFilter] = useState([]) // tags that control what the PRD includes
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')

  const loadPreview = useCallback(
    (f = filter) => api.preview(sessionId, f).then(setPreview).catch((e) => setErr(e.message)),
    [sessionId, filter],
  )

  useEffect(() => {
    api.getSession(sessionId).then(setSt).catch((e) => setErr(e.message))
  }, [sessionId])
  useEffect(() => { loadPreview() }, [loadPreview])

  // Runs an API action that returns a new session state, then refreshes the preview.
  async function act(fn) {
    setBusy(true); setErr('')
    try { const next = await fn(); if (next && next.prompt) setSt(next); await loadPreview() }
    catch (e) { setErr(e.message) }
    finally { setBusy(false) }
  }
  const refreshAll = async () => { setSt(await api.getSession(sessionId)); await loadPreview() }

  if (!st) return <div className="center-screen muted">{err || 'Loading…'}</div>
  return (
    <div className="workspace">
      <section className="pane chat-pane">
        <Chat st={st} preview={preview} busy={busy} err={err} act={act} sessionId={sessionId} />
      </section>
      <section className="pane preview-pane">
        <Preview sessionId={sessionId} st={st} preview={preview} filter={filter} setFilter={setFilter}
          onChanged={refreshAll} />
      </section>
    </div>
  )
}

function findBlock(preview, p) {
  if (!preview || p.kind !== 'question') return null
  const sec = preview.sections.find((s) => s.key === p.section)
  const item = sec && sec.items.find((i) => i.index === p.item)
  return item && item.blocks.find((b) => b.field === p.field && b.answer_id)
}

function Chat({ st, preview, busy, err, act, sessionId }) {
  const p = st.prompt
  const [text, setText] = useState('')
  const [tags, setTags] = useState([])
  const endRef = useRef(null)
  const taRef = useRef(null)
  const mic = useDictation(() => taRef.current ? taRef.current.value : '', setText)

  const flagged = findBlock(preview, p)
  const needsReview = flagged && flagged.status === 'needs_review'
  const qid = `${p.kind}:${p.section}:${p.item}:${p.field}`

  // New question -> clear the box. Flagged answer -> prefill it so it can be revised.
  useEffect(() => {
    mic.stop() // never carry dictation over to the next question
    if (needsReview) { setText(flagged.content); setTags(flagged.tags || []) }
    else { setText(''); setTags([]) }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [qid, needsReview])

  useEffect(() => { endRef.current && endRef.current.scrollIntoView({ behavior: 'smooth' }) }, [st.messages.length, busy])

  const send = () => {
    if (!text.trim() || busy) return
    mic.stop()
    // Clear the box as soon as the answer is accepted by the server (kept if the request fails).
    act(async () => {
      const next = await api.answer(sessionId, text, tags)
      setText(''); setTags([])
      return next
    })
  }

  return (
    <>
      <div className="pane-head">
        <h2>{st.session.title || 'New PRD'}</h2>
        {p.kind !== 'done' && <span className="muted">Section {p.section_index} of {p.section_count}{p.item_label ? ` · ${p.item_label}` : ''}</span>}
        {p.kind !== 'done' && <div className="progress"><div style={{ width: `${((p.section_index - 1) / p.section_count) * 100}%` }} /></div>}
      </div>

      <div className="messages">
        {st.messages.map((m) => (
          <div key={m.id} className={`msg ${m.role}`}>
            {m.role === 'user' ? <p>{m.content}</p> : <ReactMarkdown>{m.content}</ReactMarkdown>}
          </div>
        ))}
        {busy && <div className="msg agent typing"><span /><span /><span /></div>}
        <div ref={endRef} />
      </div>

      {err && <div className="note err">{err}</div>}

      {p.kind === 'repeat' && (
        <div className="composer">
          <div className="row">
            <button className="btn primary" disabled={busy} onClick={() => act(() => api.repeat(sessionId, true))}>Yes, add another {p.noun}</button>
            <button className="btn" disabled={busy} onClick={() => act(() => api.repeat(sessionId, false))}>No, that's all</button>
          </div>
        </div>
      )}

      {p.kind === 'done' && (
        <div className="composer"><p className="muted">All questions answered. Use the right-hand panel to refine, generate and download.</p></div>
      )}

      {p.kind === 'question' && (
        <div className="composer">
          {needsReview && (
            <div className="row wrap">
              {flagged.suggestion && <button className="btn" onClick={() => setText(flagged.suggestion)}>Use suggested rewrite</button>}
              <button className="btn" disabled={busy} onClick={() => act(() => api.accept(sessionId, flagged.answer_id))}>Keep as is</button>
            </div>
          )}
          <div className="inputwrap">
            <textarea ref={taRef} value={text} rows={3} disabled={busy}
              placeholder={mic.listening ? 'Listening… speak now' : 'Type your answer, or use the mic. Ctrl/⌘+Enter to send'}
              onChange={(e) => setText(e.target.value)}
              onKeyDown={(e) => { if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) send() }} />
            {mic.supported && (
              <button type="button" className={'mic ' + (mic.listening ? 'on' : '')} onClick={mic.toggle}
                title={mic.listening ? 'Stop dictation' : 'Dictate your answer'}>🎤</button>
            )}
          </div>
          {mic.error && <div className="note err">{mic.error}</div>}
          {!mic.supported && <small className="muted">Voice dictation needs Chrome, Edge or Safari.</small>}
          <TagPicker value={tags} onChange={setTags} highlightPriority={p.priority} />
          <div className="row">
            {!p.required && (
              <>
                <button className="btn" disabled={busy} onClick={() => act(() => api.skip(sessionId, 'question'))}>Skip</button>
                <button className="btn" disabled={busy} onClick={() => act(() => api.skip(sessionId, 'section'))}>Skip section</button>
              </>
            )}
            <span className="grow" />
            {p.required && <small className="muted">Required</small>}
            <button className="btn primary" disabled={busy || !text.trim()} onClick={send}>
              {needsReview ? 'Re-check answer' : 'Send'}
            </button>
          </div>
        </div>
      )}
    </>
  )
}
