import { useEffect, useRef, useState } from 'react'

// Voice dictation via the browser's Web Speech API (Chrome, Edge, Safari).
export function useDictation(getText, setText) {
  const SR = typeof window !== 'undefined' && (window.SpeechRecognition || window.webkitSpeechRecognition)
  const [listening, setListening] = useState(false)
  const [error, setError] = useState('')
  const rec = useRef(null)
  const base = useRef('')

  // Fully detach the recognizer so late results can never write into the box again.
  function stop() {
    const r = rec.current
    if (r) {
      r.onresult = null; r.onend = null; r.onerror = null
      try { r.abort() } catch { /* already stopped */ }
      rec.current = null
    }
    setListening(false)
  }

  useEffect(() => () => stop(), [])  // eslint-disable-line react-hooks/exhaustive-deps

  function toggle() {
    if (!SR) return
    if (listening) {
      stop()
      return
    }
    setError('')
    const r = new SR()
    r.continuous = true
    r.interimResults = true
    r.lang = navigator.language || 'en-US'
    base.current = getText()
    r.onresult = (e) => {
      let t = ''
      for (let i = 0; i < e.results.length; i++) t += e.results[i][0].transcript
      const b = base.current
      setText(b + (b && !/\s$/.test(b) ? ' ' : '') + t.trim())
    }
    r.onerror = (e) => {
      setError(e.error === 'not-allowed' ? 'Microphone permission was denied.' : `Voice error: ${e.error}`)
      setListening(false)
    }
    r.onend = () => { rec.current = null; setListening(false) }
    rec.current = r
    r.start()
    setListening(true)
  }
  return { supported: !!SR, listening, toggle, stop, error }
}
