import { X } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { loadArtifact, type ArtifactResponse } from '../api/client'
import type { SelectableRecord } from '../features/workbench'
import { JsonView } from './JsonView'

type InspectorTab = 'json' | 'related' | 'file'

interface InspectorProps {
  activeRecord: SelectableRecord
  open: boolean
  onClose: () => void
  onResume: (sessionID: string) => void
}

interface ArtifactPreview {
  path: string
  loading?: boolean
  artifact?: ArtifactResponse
  error?: string
}

interface DetailItem {
  label: string
  value: string
}

const inspectorTabs: InspectorTab[] = ['json', 'related', 'file']

export function Inspector({ activeRecord, open, onClose, onResume }: InspectorProps) {
  const [inspectorTab, setInspectorTab] = useState<InspectorTab>('json')
  const [artifactPreview, setArtifactPreview] = useState<ArtifactPreview | undefined>()
  const relatedItems = useMemo(() => relatedDetails(activeRecord.source), [activeRecord.source])
  const textItems = useMemo(() => textDetails(activeRecord.source), [activeRecord.source])

  useEffect(() => {
    setInspectorTab('json')
    if (activeRecord.kind !== 'artifact') setArtifactPreview(undefined)
  }, [activeRecord.id, activeRecord.kind])

  async function fetchArtifact(path: string) {
    setArtifactPreview({ path, loading: true })
    try {
      setArtifactPreview({ path, artifact: await loadArtifact(path) })
    } catch (error) {
      setArtifactPreview({ path, error: error instanceof Error ? error.message : String(error) })
    }
  }

  return (
    <aside className={open ? 'inspector open' : 'inspector'} aria-label="Inspector">
      <header>
        <div>
          <span>{activeRecord.kind}</span>
          <strong>{activeRecord.title}</strong>
        </div>
        <button type="button" onClick={onClose} title="Close inspector">
          <X size={16} />
        </button>
      </header>
      {activeRecord.subtitle ? <p>{activeRecord.subtitle}</p> : null}
      {activeRecord.kind === 'session' ? (
        <button className="primary" type="button" onClick={() => onResume(activeRecord.title)}>
          Resume session
        </button>
      ) : null}
      {activeRecord.kind === 'artifact' ? (
        <ArtifactLoader
          path={activeRecord.title}
          preview={artifactPreview?.path === activeRecord.title ? artifactPreview : undefined}
          onLoad={fetchArtifact}
        />
      ) : null}
      <nav className="inspector-tabs" aria-label="Inspector details">
        {inspectorTabs.map((name) => (
          <button key={name} className={inspectorTab === name ? 'active' : ''} type="button" onClick={() => setInspectorTab(name)}>
            {inspectorTabLabel(name)}
          </button>
        ))}
      </nav>
      <section className="inspector-panel">
        {inspectorTab === 'json' ? <JsonView value={activeRecord.source} /> : null}
        {inspectorTab === 'related' ? <RelatedPanel items={relatedItems} /> : null}
        {inspectorTab === 'file' ? (
          <FileTextPanel items={textItems} artifact={artifactPreview?.path === activeRecord.title ? artifactPreview?.artifact : undefined} />
        ) : null}
      </section>
    </aside>
  )
}

function inspectorTabLabel(tab: InspectorTab): string {
  switch (tab) {
    case 'json':
      return 'Full JSON'
    case 'related':
      return 'Related'
    case 'file':
      return 'File/Text'
  }
}

function RelatedPanel({ items }: { items: DetailItem[] }) {
  if (items.length === 0) return <p className="empty-text">No related identifiers.</p>
  return (
    <dl className="detail-list">
      {items.map((item) => (
        <div key={`${item.label}:${item.value}`}>
          <dt>{item.label}</dt>
          <dd>{item.value}</dd>
        </div>
      ))}
    </dl>
  )
}

function FileTextPanel({ items, artifact }: { items: DetailItem[]; artifact: ArtifactResponse | undefined }) {
  if (!artifact && items.length === 0) return <p className="empty-text">No file or text fields.</p>
  return (
    <div className="text-detail-list">
      {artifact ? (
        <section>
          <h2>{artifact.path}</h2>
          <pre>{artifact.content}</pre>
        </section>
      ) : null}
      {items.map((item) => (
        <section key={`${item.label}:${item.value.slice(0, 32)}`}>
          <h2>{item.label}</h2>
          <pre>{item.value}</pre>
        </section>
      ))}
    </div>
  )
}

function ArtifactLoader({
  path,
  preview,
  onLoad
}: {
  path: string
  preview: ArtifactPreview | undefined
  onLoad: (path: string) => void
}) {
  return (
    <section className="artifact-loader">
      <button className="primary" type="button" onClick={() => onLoad(path)} disabled={preview?.loading}>
        {preview?.loading ? 'Loading artifact' : 'Load artifact'}
      </button>
      {preview?.error ? <p className="inline-error">{preview.error}</p> : null}
      {preview?.artifact ? <JsonView value={preview.artifact} /> : null}
    </section>
  )
}

function relatedDetails(value: unknown): DetailItem[] {
  const candidates = collectKeyedStrings(value)
  const keys = ['tool_call_id', 'call_id', 'session_id', 'trace_id', 'state_id', 'id']
  return uniqueDetails(candidates.filter((item) => keys.includes(item.label.toLowerCase())))
}

function textDetails(value: unknown): DetailItem[] {
  const candidates = collectKeyedStrings(value)
  const keys = ['displaytext', 'display_text', 'text', 'content', 'stdout', 'stderr', 'output']
  return uniqueDetails(candidates.filter((item) => keys.includes(item.label.toLowerCase())))
}

function collectKeyedStrings(value: unknown, items: DetailItem[] = []): DetailItem[] {
  if (!value || typeof value !== 'object') return items
  if (Array.isArray(value)) {
    for (const item of value) collectKeyedStrings(item, items)
    return items
  }
  for (const [key, raw] of Object.entries(value)) {
    if (typeof raw === 'string' && raw.trim()) {
      items.push({ label: key, value: raw })
      continue
    }
    collectKeyedStrings(raw, items)
  }
  return items
}

function uniqueDetails(items: DetailItem[]): DetailItem[] {
  const seen = new Set<string>()
  return items.filter((item) => {
    const key = `${item.label}:${item.value}`
    if (seen.has(key)) return false
    seen.add(key)
    return true
  })
}
