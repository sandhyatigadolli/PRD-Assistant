import { useState } from 'react'
import ReactMarkdown from 'react-markdown'
import { api } from '../api'
import { TagBadges, useTags } from './TagPicker.jsx'
import EditModal from './EditModal.jsx'

export default function Preview({ sessionId, st, preview, filter, setFilter, onChanged }) {
  const [tab, setTab] = useState('live')
  const [editing, setEditing] = useState(null)
  const [generating, setGenerating] = useState(false)
  const [err, setErr] = useState('')
  const tags = useTags()
  const polished = st.session.generated
  const source = tab === 'polished' ? 'polished' : 'answers'

  const toggle = (k) => setFilter(filter.includes(k) ? filter.filter((x) => x !== k) : [...filter, k])

  async function generate() {
    setGenerating(true); setErr('')
    try { await api.generate(sessionId, filter); await onChanged(); setTab('polished') }
    catch (e) { setErr(e.message) }
    finally { setGenerating(false) }
  }

  return (
    <>
      <div className="pane-head">
        <div className="tabs">
          <button className={tab === 'live' ? 'on' : ''} onClick={() => setTab('live')}>Live preview</button>
          <button className={tab === 'polished' ? 'on' : ''} onClick={() => setTab('polished')}>Polished PRD</button>
        </div>
        <div className="row wrap exportbar">
          <button className="btn" disabled={generating} onClick={generate}>
            {generating ? 'Generating…' : polished ? 'Regenerate polished PRD' : 'Generate polished PRD'}
          </button>
          <a className={'btn primary' + (source === 'polished' && !polished ? ' disabled' : '')}
            href={api.exportUrl(sessionId, 'docx', source, filter)}>⬇ Word</a>
          <a className={'btn primary' + (source === 'polished' && !polished ? ' disabled' : '')}
            href={api.exportUrl(sessionId, 'pdf', source, filter)}>⬇ PDF</a>
        </div>
        <div className="filter">
          <span className="muted">Include tags:</span>
          {tags.map((t) => (
            <button key={t.key} title={t.description} className={`chip small tag-${t.group} ${filter.includes(t.key) ? 'on' : ''}`}
              onClick={() => toggle(t.key)}>{t.key}</button>
          ))}
          {filter.length > 0 && <button className="link" onClick={() => setFilter([])}>clear</button>}
        </div>
        {filter.length > 0 && <small className="muted">Showing untagged content plus blocks tagged {filter.join(' / ')}. Hidden blocks are greyed out.</small>}
        {err && <div className="note err">{err}</div>}
      </div>

      <div className="doc">
        {tab === 'polished' ? (
          polished
            ? <div className="polished"><ReactMarkdown>{polished}</ReactMarkdown></div>
            : <p className="muted empty">No polished PRD yet. Answer the required questions, then click “Generate polished PRD”. The AI will turn your answers into a structured document.</p>
        ) : !preview ? <p className="muted">Loading…</p> : (
          <>
            <h1 className="doc-title">{preview.title}</h1>
            <p className="muted hint">Click any block to edit it. You'll see a before/after diff before anything changes.</p>
            {preview.sections.map((sec) => (
              <div className="doc-section" key={sec.key}>
                <h2>{sec.title}</h2>
                {sec.items.map((it) => (
                  <div key={it.index}>
                    {it.label && <h3>{it.label}</h3>}
                    {it.blocks.map((b) => <Block key={b.field} b={b} onEdit={() => setEditing(b)} />)}
                  </div>
                ))}
              </div>
            ))}
          </>
        )}
      </div>

      {editing && (
        <EditModal sessionId={sessionId} block={editing} onClose={() => setEditing(null)}
          onSaved={async () => { setEditing(null); await onChanged() }} />
      )}
    </>
  )
}

function Block({ b, onEdit }) {
  if (b.pending)
    return <div className="block pending"><span className="label">{b.label}</span><span className="muted">Not answered yet{b.required ? ' · required' : ''}</span></div>
  const empty = b.skipped || !b.content.trim()
  return (
    <div className={'block' + (b.status === 'needs_review' ? ' warn' : '') + (!b.included ? ' dim' : '')}
      role="button" tabIndex={0} onClick={onEdit} onKeyDown={(e) => e.key === 'Enter' && onEdit()}>
      <div className="block-head">
        <span className="label">{b.label}</span>
        <TagBadges tags={b.tags} />
        {b.status === 'needs_review' && <span className="chip small warn">needs review</span>}
        {!b.included && <span className="chip small">hidden by filter</span>}
        <span className="edit-hint">✎ edit</span>
      </div>
      {empty ? <div className="muted">Skipped. Click to add.</div> : <div className="content">{b.content}</div>}
    </div>
  )
}
