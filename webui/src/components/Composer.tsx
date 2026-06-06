import { Send } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { loadCompletions, type CompletionItem } from '../api/client'

interface ComposerProps {
  value: string
  running: boolean
  error: string
  onChange: (value: string) => void
  onSubmit: (value: string) => void
}

export function Composer({ value, running, error, onChange, onSubmit }: ComposerProps) {
  const textareaRef = useRef<HTMLTextAreaElement | null>(null)
  const [completions, setCompletions] = useState<CompletionItem[]>([])
  const [completionError, setCompletionError] = useState('')

  useEffect(() => {
    if (running || !value.startsWith('/') || value.includes('\n')) {
      setCompletions([])
      setCompletionError('')
      return
    }
    let active = true
    const timer = window.setTimeout(async () => {
      try {
        const result = await loadCompletions(value)
        if (!active) return
        setCompletions(result.items)
        setCompletionError('')
      } catch (err) {
        if (!active) return
        setCompletions([])
        setCompletionError(err instanceof Error ? err.message : String(err))
      }
    }, 120)
    return () => {
      active = false
      window.clearTimeout(timer)
    }
  }, [running, value])

  function applyCompletion(item: CompletionItem) {
    onChange(item.replacement)
    setCompletions([])
    window.requestAnimationFrame(() => textareaRef.current?.focus())
  }

  return (
    <form
      className="composer"
      onSubmit={(event) => {
        event.preventDefault()
        const text = value.trim()
        if (text) onSubmit(text)
      }}
    >
      {completions.length > 0 ? (
        <div className="completion-list" aria-label="Slash completions">
          {completions.map((item) => (
            <button key={item.id} type="button" onClick={() => applyCompletion(item)}>
              <strong>{item.label}</strong>
              {item.detail ? <span>{item.detail}</span> : null}
            </button>
          ))}
        </div>
      ) : null}
      <textarea
        id="pragma-composer-input"
        name="prompt"
        ref={textareaRef}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === 'Enter' && !event.shiftKey) {
            event.preventDefault()
            event.currentTarget.form?.requestSubmit()
          }
        }}
        placeholder="Start a message or run a slash command."
        disabled={running}
        rows={2}
      />
      <button className="primary icon-button" type="submit" disabled={running || !value.trim()} title="Send">
        <Send size={17} />
      </button>
      {error ? <p className="inline-error">{error}</p> : null}
      {completionError ? <p className="inline-error">{completionError}</p> : null}
    </form>
  )
}
