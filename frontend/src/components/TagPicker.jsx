import { useEffect, useState } from 'react'
import { api } from '../api'

let cache = null
export function useTags() {
  const [tags, setTags] = useState(cache || [])
  useEffect(() => { if (!cache) api.tags().then((t) => { cache = t; setTags(t) }).catch(() => {}) }, [])
  return tags
}

const GROUP = { priority: 'Priority', release: 'Release', status: 'Status' }

// Multi-select tag chips grouped by type (priority / release / status).
export default function TagPicker({ value, onChange, highlightPriority }) {
  const tags = useTags()
  const toggle = (k) => onChange(value.includes(k) ? value.filter((x) => x !== k) : [...value, k])
  return (
    <div className="tagpicker">
      {Object.keys(GROUP).map((g) => (
        <div className={'taggroup' + (g === 'priority' && highlightPriority ? ' need' : '')} key={g}>
          <span className="taggroup-label">{GROUP[g]}{g === 'priority' && highlightPriority ? ' *' : ''}</span>
          {tags.filter((t) => t.group === g).map((t) => (
            <button type="button" key={t.key} title={t.description}
              className={`chip tag-${g} ${value.includes(t.key) ? 'on' : ''}`} onClick={() => toggle(t.key)}>
              {t.key}
            </button>
          ))}
        </div>
      ))}
    </div>
  )
}

export function TagBadges({ tags }) {
  const all = useTags()
  const group = (k) => (all.find((t) => t.key === k) || {}).group || 'status'
  return <>{(tags || []).map((t) => <span key={t} className={`chip small on tag-${group(t)}`}>{t}</span>)}</>
}
