// Thin wrapper around the Go REST API. Errors carry the server's message (+ hint).
async function call(method, path, body) {
  const res = await fetch('/api' + path, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  })
  const data = await res.json().catch(() => ({}))
  if (!res.ok) {
    const e = new Error(data.error || `Request failed (${res.status})`)
    e.hint = data.hint
    throw e
  }
  return data
}

const qs = (tags) => (tags && tags.length ? `tags=${encodeURIComponent(tags.join(','))}` : '')

export const api = {
  llmStatus: () => call('GET', '/llm/status'),
  llmLocal: (url) => call('GET', '/llm/local' + (url ? `?base_url=${encodeURIComponent(url)}` : '')),
  llmConfigure: (cfg) => call('POST', '/llm/configure', cfg),
  llmReset: () => call('POST', '/llm/reset'),
  tags: () => call('GET', '/tags'),
  prdTypes: () => call('GET', '/prd-types'),
  sessions: () => call('GET', '/sessions'),
  createSession: (prd_type) => call('POST', '/sessions', { prd_type }),
  getSession: (id) => call('GET', `/sessions/${id}`),
  answer: (id, content, tags) => call('POST', `/sessions/${id}/answer`, { content, tags }),
  skip: (id, scope) => call('POST', `/sessions/${id}/skip`, { scope }),
  repeat: (id, add) => call('POST', `/sessions/${id}/repeat`, { add }),
  accept: (id, aid) => call('POST', `/sessions/${id}/answers/${aid}/accept`),
  propose: (id, aid, body) => call('POST', `/sessions/${id}/answers/${aid}/propose`, body),
  apply: (id, aid, content, tags) => call('POST', `/sessions/${id}/answers/${aid}/apply`, { content, tags }),
  history: (id, aid) => call('GET', `/sessions/${id}/answers/${aid}/history`),
  preview: (id, tags) => call('GET', `/sessions/${id}/preview?${qs(tags)}`),
  generate: (id, tags) => call('POST', `/sessions/${id}/generate?${qs(tags)}`),
  exportUrl: (id, format, source, tags) =>
    `/api/sessions/${id}/export?format=${format}&source=${source}&${qs(tags)}`,
}
