import { useState } from 'react'
import { api } from '../api'

const PROVIDERS = {
  gemini: { label: 'Google Gemini: FREE tier (recommended)', placeholder: 'AIza…', link: 'https://aistudio.google.com/apikey' },
  groq: { label: 'Groq: FREE tier, very fast', placeholder: 'gsk_…', link: 'https://console.groq.com/keys' },
  openrouter: { label: 'OpenRouter: free models', placeholder: 'sk-or-…', link: 'https://openrouter.ai/keys' },
  openai: { label: 'OpenAI (ChatGPT API): paid, needs credit', placeholder: 'sk-…', link: 'https://platform.openai.com/api-keys' },
  anthropic: { label: 'Anthropic (Claude API): paid, needs credit', placeholder: 'sk-ant-…', link: 'https://console.anthropic.com/settings/keys' },
}

export default function LLMGate({ defaultLocalUrl, onReady }) {
  const [choice, setChoice] = useState(null) // null | api | local
  const [blocked, setBlocked] = useState(null) // {error, hint}
  const [busy, setBusy] = useState(false)

  // API form
  const [provider, setProvider] = useState('gemini')
  const [apiKey, setApiKey] = useState('')
  const [model, setModel] = useState('')
  // Local form
  const [url, setUrl] = useState(defaultLocalUrl || 'http://localhost:11434')
  const [models, setModels] = useState(null)
  const [localModel, setLocalModel] = useState('')
  const [localMsg, setLocalMsg] = useState(null)

  async function connect(cfg) {
    setBusy(true); setBlocked(null)
    try { await api.llmConfigure(cfg); onReady() }
    catch (e) { setBlocked({ error: e.message, hint: e.hint }) }
    finally { setBusy(false) }
  }

  async function checkLocal() {
    setBusy(true); setLocalMsg(null); setModels(null)
    try {
      const r = await api.llmLocal(url)
      if (r.reachable) { setModels(r.models); setLocalModel(r.models[0]); setLocalMsg({ ok: true, text: `Connected. ${r.models.length} model(s) found.` }) }
      else setLocalMsg({ ok: false, text: r.error })
    } catch (e) { setLocalMsg({ ok: false, text: e.message }) }
    finally { setBusy(false) }
  }

  return (
    <div className="center-screen">
      <div className="card gate">
        <div className="brand big"><span className="logo">PRD</span> Assistant</div>
        <h1>First, let's connect an LLM</h1>
        <p className="muted">
          I need a language model to review your answers and write the final PRD. Either set an API key
          or use an LLM running on your machine. One of the two is required to continue.
        </p>

        {!choice && (
          <div className="choices">
            <button className="choice" onClick={() => setChoice('api')}>
              <b>🔑 Use an API key</b>
              <span>Free options: Gemini, Groq, OpenRouter (nothing to download). The key stays in server memory and is never saved.</span>
            </button>
            <button className="choice" onClick={() => { setChoice('local'); checkLocal() }}>
              <b>💻 Use a local LLM</b>
              <span>Ollama or LM Studio already running on your computer. Free and private.</span>
            </button>
          </div>
        )}

        {choice === 'api' && (
          <form className="form" onSubmit={(e) => { e.preventDefault(); connect({ mode: 'api', provider, api_key: apiKey, model }) }}>
            <label>Provider
              <select value={provider} onChange={(e) => setProvider(e.target.value)}>
                {Object.entries(PROVIDERS).map(([k, v]) => <option key={k} value={k}>{v.label}</option>)}
              </select>
            </label>
            <label>API key
              <input type="password" autoComplete="off" value={apiKey} placeholder={PROVIDERS[provider].placeholder}
                onChange={(e) => setApiKey(e.target.value)} />
              <small className="muted">Get a key at <a href={PROVIDERS[provider].link} target="_blank" rel="noreferrer">{PROVIDERS[provider].link.replace('https://', '')}</a></small>
            </label>
            <label>Model <span className="muted">(optional, a sensible default is used)</span>
              <input value={model} onChange={(e) => setModel(e.target.value)} />
            </label>
            <div className="row">
              <button type="button" className="btn" onClick={() => { setChoice(null); setBlocked(null) }}>Back</button>
              <button className="btn primary" disabled={busy || !apiKey.trim()}>{busy ? 'Testing key…' : 'Save & test'}</button>
            </div>
          </form>
        )}

        {choice === 'local' && (
          <div className="form">
            <label>Local LLM server URL
              <input value={url} onChange={(e) => setUrl(e.target.value)} />
            </label>
            <button className="btn" onClick={checkLocal} disabled={busy}>{busy && !models ? 'Checking…' : 'Check connection'}</button>
            {localMsg && <div className={'note ' + (localMsg.ok ? 'ok' : 'err')}>{localMsg.text}</div>}
            {models && (
              <label>Model
                <select value={localModel} onChange={(e) => setLocalModel(e.target.value)}>
                  {models.map((m) => <option key={m}>{m}</option>)}
                </select>
              </label>
            )}
            <div className="row">
              <button className="btn" onClick={() => { setChoice(null); setBlocked(null) }}>Back</button>
              <button className="btn primary" disabled={busy || !models}
                onClick={() => connect({ mode: 'local', base_url: url, model: localModel })}>
                {busy && models ? 'Testing model…' : 'Connect'}
              </button>
            </div>
          </div>
        )}

        {blocked && (
          <div className="blocked">
            <b>Can't continue yet.</b>
            <p>{blocked.error}</p>
            <p>{blocked.hint || 'Either an API key or a running local LLM is required to build a PRD.'}</p>
          </div>
        )}
      </div>
    </div>
  )
}
