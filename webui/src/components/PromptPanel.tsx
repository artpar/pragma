import { useEffect, useState } from 'react'
import { answerAsk, answerPermission } from '../api/client'
import { JsonView } from './JsonView'
import type { AskField, PromptProjection, PromptStatus } from '../features/workbench'

interface PromptPanelProps {
  prompt: PromptProjection
}

export function PromptPanel({ prompt }: PromptPanelProps) {
  if (prompt.kind === 'permission') {
    return <PermissionPanel prompt={prompt} />
  }
  if (prompt.kind === 'ask') {
    return <AskPanel prompt={prompt} />
  }
  return null
}

function PermissionPanel({ prompt }: PromptPanelProps) {
  const [localState, setLocalState] = useState<LocalPromptState>('idle')
  const [message, setMessage] = useState('')
  const state = localState === 'unavailable' ? 'unavailable' : prompt.status
  const canRespond = state === 'pending' && localState !== 'submitting'

  useEffect(() => {
    setLocalState('idle')
    setMessage('')
  }, [prompt.id])

  async function decide(decision: 'allow' | 'deny', scope: 'none' | 'session' = 'none') {
    setLocalState('submitting')
    setMessage('')
    try {
      await answerPermission(prompt.id, decision, scope)
      setMessage(`${decision} submitted`)
    } catch (error) {
      const detail = error instanceof Error ? error.message : String(error)
      setMessage(detail)
      setLocalState(detail.toLowerCase().includes('not found') ? 'unavailable' : 'idle')
    }
  }

  return (
    <section className="blocking-panel" aria-label="Permission request">
      <div className="request-summary">
        <h2>Permission request</h2>
        <StatusBadge status={state} />
        <p>{prompt.subtitle}</p>
      </div>
      <div className="decision-row">
        <button className="primary" type="button" onClick={() => decide('allow')} disabled={!canRespond || !prompt.id}>
          Allow once
        </button>
        <button type="button" onClick={() => decide('allow', 'session')} disabled={!canRespond || !prompt.id}>
          Allow this session
        </button>
        <button type="button" onClick={() => decide('deny')} disabled={!canRespond || !prompt.id}>
          Deny once
        </button>
      </div>
      <JsonView value={prompt.requestEvent} />
      {message ? <p className={localState === 'unavailable' ? 'state-note warning' : 'state-note'}>{message}</p> : null}
      {prompt.resolutionEvent ? <JsonView value={prompt.resolutionEvent} defaultExpanded={false} /> : null}
    </section>
  )
}

function AskPanel({ prompt }: PromptPanelProps) {
  const [answers, setAnswers] = useState<Record<string, string>>({})
  const [localState, setLocalState] = useState<LocalPromptState>('idle')
  const [message, setMessage] = useState('')
  const state = localState === 'unavailable' ? 'unavailable' : prompt.status
  const canRespond = state === 'pending' && localState !== 'submitting'
  const hasAnswer = prompt.fields.some((field) => answers[field.key]?.trim())

  useEffect(() => {
    setAnswers({})
    setLocalState('idle')
    setMessage('')
  }, [prompt.id])

  async function submit() {
    setLocalState('submitting')
    setMessage('')
    try {
      await answerAsk(prompt.id, normalizedAnswers(prompt.fields, answers))
      setMessage('answer submitted')
    } catch (error) {
      const detail = error instanceof Error ? error.message : String(error)
      setMessage(detail)
      setLocalState(detail.toLowerCase().includes('not found') ? 'unavailable' : 'idle')
    }
  }

  return (
    <section className="blocking-panel" aria-label="Ask request">
      <div className="request-summary">
        <h2>Ask request</h2>
        <StatusBadge status={state} />
        <p>{prompt.subtitle}</p>
      </div>
      <div className="field-stack">
        {prompt.fields.map((field) => (
          <AskFieldControl key={field.key} field={field} value={answers[field.key] ?? ''} onChange={(value) => setAnswers((current) => ({ ...current, [field.key]: value }))} disabled={!canRespond} />
        ))}
      </div>
      <div className="decision-row">
        <button className="primary" type="button" onClick={submit} disabled={!canRespond || !prompt.id || !hasAnswer}>
          Submit answer
        </button>
      </div>
      <JsonView value={prompt.requestEvent} />
      {message ? <p className={localState === 'unavailable' ? 'state-note warning' : 'state-note'}>{message}</p> : null}
      {prompt.resolutionEvent ? <JsonView value={prompt.resolutionEvent} defaultExpanded={false} /> : null}
    </section>
  )
}

type LocalPromptState = 'idle' | 'submitting' | 'unavailable'

function StatusBadge({ status }: { status: PromptStatus | 'unavailable' }) {
  return <span className={`status-pill ${status}`}>{status}</span>
}

function AskFieldControl({
  field,
  value,
  onChange,
  disabled
}: {
  field: AskField
  value: string
  onChange: (value: string) => void
  disabled: boolean
}) {
  return (
    <label className="ask-field">
      <span>{field.header || field.label}</span>
      <strong>{field.label}</strong>
      {field.options.length > 0 ? (
        <div className="option-row">
          {field.options.map((option) => (
            <button
              key={option.label}
              className={selectedOption(value, option.label) ? 'active' : ''}
              type="button"
              onClick={() => onChange(nextOptionValue(value, option.label, field.multiSelect))}
              disabled={disabled}
              title={option.description || option.label}
            >
              {option.label}
            </button>
          ))}
        </div>
      ) : null}
      <textarea value={value} onChange={(event) => onChange(event.target.value)} placeholder="Answer" rows={2} disabled={disabled} />
    </label>
  )
}

function normalizedAnswers(fields: AskField[], answers: Record<string, string>): Record<string, string> {
  return Object.fromEntries(fields.map((field) => [field.key, answers[field.key] ?? '']))
}

function selectedOption(value: string, label: string): boolean {
  return value.split(',').map((part) => part.trim()).includes(label)
}

function nextOptionValue(value: string, label: string, multiSelect: boolean): string {
  if (!multiSelect) return label
  const selected = new Set(value.split(',').map((part) => part.trim()).filter(Boolean))
  if (selected.has(label)) selected.delete(label)
  else selected.add(label)
  return Array.from(selected).join(', ')
}
