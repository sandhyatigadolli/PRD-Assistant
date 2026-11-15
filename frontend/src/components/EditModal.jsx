import { useEffect, useRef, useState } from 'react'
import { api } from '../api'
import { useDictation } from '../useDictation'
import TagPicker, { TagBadges } from './TagPicker.jsx'

// Edit flow, like a GitHub PR: change -> review before/after diff -> confirm.
export default function EditModal({ sessionId, block, onClose, onSaved }) {
  const [text, setText] = useState(block.content)
  const [tags, setTags] = useState(block.tags || [])
  const [instruction, setInstruction] = useState('')
  const [step, setStep] = useState('edit') // edit | review
  const [proposal, setProposal] = useState(null)
  const [history, setHistory] = useState([])
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const taRef = useRef(null)
  const mic = useDictation(() => (taRef.current ? taRef.current.value : ''), setText)

  useEffect(() => { api.history(sessionId, block.answer_id).then(setHistory).catch(() => {}) }, [sessionId, block.answer_id])

  const run = async (fn) => { setBusy(true); setErr(''); try { await fn() } catch (e) { setErr(e.message) } finally { setBusy(false) } }

  const review = () => run(async () => {
    setProposal(await api.propose(sessionId, block.answer_id, { content: text, tags }))
    setStep('review')
  })
  const askAI = () => run(async () => {
    const p = await api.propose(sessionId, block.answer_id, { instruction, tags })
    setText(p.new); setInstruction('')
  })
  const confirm = () => run(async () => { await api.apply(sessionId, block.answer_id, proposal.new, proposal.new_tags); await onSaved() })

  return (
    <div className="modal-back" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="modal">
        <div className="modal-head">
          <h3>{step === 'review' ? 'Review changes: ' : 'Edit: '}{block.label}</h3>
          <button className="link" onClick={onClose}>✕</button>
        </div>
        {err && <div className="note err">{err}</div>}

        {step === 'edit' && (
          <>
            <div className="ai-row">
              <input placeholder="Ask AI to change it, e.g. “make the goal measurable”" value={instruction}
                onChange={(e) => setInstruction(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && instruction.trim() && askAI()} />
              <button className="btn" disabled={busy || !instruction.trim()} onClick={askAI}>{busy ? '…' : 'Ask AI'}</button>
            </div>
            <div className="inputwrap">
              <textarea ref={taRef} rows={8} value={text} onChange={(e) => setText(e.target.value)} />
              {mic.supported && <button type="button" className={'mic ' + (mic.listening ? 'on' : '')} onClick={mic.toggle}>🎤</button>}
            </div>
            <TagPicker value={tags} onChange={setTags} />
            <div className="row">
              <span className="grow" />
              <button className="btn" onClick={onClose}>Cancel</button>
              <button className="btn primary" disabled={busy} onClick={review}>Review changes</button>
            </div>

            <div className="history">
              <h4>Recent changes <span className="muted">(last 3)</span></h4>
              {history.length === 0 && <p className="muted">No earlier versions yet.</p>}
              {history.map((h) => (
                <div className="hist-item" key={h.id}>
                  <div className="hist-meta">
                    <span className="muted">{new Date(h.created_at).toLocaleString()}</span>
                    <TagBadges tags={h.tags} />
                    <button className="link" onClick={() => { setText(h.content); setTags(h.tags || []) }}>Restore this version</button>
                  </div>
                  <div className="hist-content">{h.content || <i className="muted">(empty)</i>}</div>
                </div>
              ))}
            </div>
          </>
        )}

        {step === 'review' && proposal && (
          <>
            {!proposal.changed && <div className="note">Nothing changed.</div>}
            <div className="diff">
              {proposal.diff.map((s, i) => <span key={i} className={'d-' + s.op}>{s.text}</span>)}
            </div>
            <div className="split">
              <div><h4>Before</h4><div className="side del-side">{proposal.old || <i className="muted">(empty)</i>}</div><TagBadges tags={proposal.old_tags} /></div>
              <div><h4>After</h4><div className="side add-side">{proposal.new || <i className="muted">(empty)</i>}</div><TagBadges tags={proposal.new_tags} /></div>
            </div>
            <div className="row">
              <button className="btn" onClick={() => setStep('edit')}>Back</button>
              <span className="grow" />
              <button className="btn primary" disabled={busy || !proposal.changed} onClick={confirm}>Confirm update</button>
            </div>
          </>
        )}
      </div>
    </div>
  )
}
