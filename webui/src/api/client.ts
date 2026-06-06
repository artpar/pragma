export type JsonValue = unknown

export type RawRecord = Record<string, JsonValue>

export interface EventEnvelope {
  sequence: number
  received_at: string
  type: string
  data_type: string
  data: JsonValue
}

export interface RuntimeState {
  version?: string
  workspace?: string
  model?: string
  provider?: string
  mcp_servers?: JsonValue
  session_start?: string
  running?: boolean
}

export interface StateResponse {
  runtime?: RuntimeState
  app_state?: RawRecord
  prompt_history?: JsonValue
}

export interface SessionSummary extends RawRecord {
  id: string
  model?: string
  provider?: string
  work_dir?: string
  turn_count?: number
  cost_usd?: number
  updated_at?: string
  in_current_work_dir?: boolean
}

export type SessionRecord = RawRecord

export interface PageMeta extends RawRecord {
  number?: number
  size?: number
  total?: number
  pages?: number
  start?: number
  end?: number
}

export interface CollectionLinks extends RawRecord {
  self?: string
  first?: string
  prev?: string
  next?: string
  last?: string
}

export interface CollectionResult<T> {
  items: T[]
  links: CollectionLinks
  page?: PageMeta
}

export interface EventResource extends RawRecord {
  envelope: EventEnvelope
}

export interface ArtifactResponse {
  path: string
  content: string
}

export interface CompletionItem extends RawRecord {
  id: string
  label: string
  detail?: string
  replacement: string
}

interface JsonAPIResource<T = JsonValue> {
  type: string
  id?: string
  attributes?: T
  meta?: JsonValue
}

interface JsonAPIDocument<T = JsonValue> {
  data?: JsonAPIResource<T> | JsonAPIResource<T>[] | null
  errors?: Array<{ status?: string; title?: string; detail?: string }>
  links?: JsonValue
  meta?: JsonValue
  jsonapi?: { version?: string }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const document = await requestDocument<T>(path, init)
  return unwrapJSONAPI<T>(document)
}

async function requestDocument<T>(path: string, init?: RequestInit): Promise<JsonAPIDocument<T>> {
  const headers: Record<string, string> = {
    Accept: 'application/vnd.api+json',
    ...(init?.headers as Record<string, string> | undefined)
  }
  if (init?.body) headers['Content-Type'] = 'application/vnd.api+json'
  const response = await fetch(path, {
    ...init,
    headers
  })
  const document = (await response.json()) as JsonAPIDocument<T>
  if (!response.ok) {
    const detail = document.errors?.map((error) => error.detail || error.title).filter(Boolean).join('; ')
    throw new Error(detail || `${response.status} ${response.statusText}`)
  }
  return document
}

function unwrapJSONAPI<T>(document: JsonAPIDocument<T>): T {
  if (Array.isArray(document.data)) {
    return document.data.map((resource) => ({ id: resource.id, ...(resource.attributes as RawRecord) })) as T
  }
  if (document.data && typeof document.data === 'object') {
    return (document.data.attributes ?? ({} as T)) as T
  }
  return {} as T
}

function unwrapJSONAPICollection<T>(document: JsonAPIDocument<T>): CollectionResult<T> {
  const data = Array.isArray(document.data) ? document.data : []
  const page = typeof document.meta === 'object' && document.meta && 'page' in document.meta
    ? ((document.meta as RawRecord).page as PageMeta)
    : undefined
  return {
    items: data.map((resource) => ({ id: resource.id, ...(resource.attributes as RawRecord) })) as T[],
    links: objectRecord(document.links) as CollectionLinks,
    page
  }
}

function objectRecord(value: JsonValue): RawRecord {
  return value && typeof value === 'object' && !Array.isArray(value) ? (value as RawRecord) : {}
}

function resourceBody(type: string, attributes: Record<string, unknown>, id?: string): string {
  return JSON.stringify({
    data: {
      type,
      ...(id ? { id } : {}),
      attributes
    }
  })
}

export function loadState(): Promise<StateResponse> {
  return request<StateResponse>('/api/state')
}

export async function loadSessions(path = '/api/sessions'): Promise<CollectionResult<SessionSummary>> {
  const document = await requestDocument<SessionSummary>(path)
  return unwrapJSONAPICollection<SessionSummary>(document)
}

export function loadSession(sessionId: string): Promise<SessionRecord> {
  return request<SessionRecord>(`/api/sessions/${encodeURIComponent(sessionId)}`)
}

export async function loadCompletions(query: string): Promise<CollectionResult<CompletionItem>> {
  const document = await requestDocument<CompletionItem>(`/api/completions?q=${encodeURIComponent(query)}`)
  return unwrapJSONAPICollection<CompletionItem>(document)
}

export async function loadRecentEvents(
  path = '/api/events/recent?page[number]=1&page[size]=200'
): Promise<CollectionResult<EventEnvelope>> {
  const document = await requestDocument<EventResource>(path)
  const collection = unwrapJSONAPICollection<EventResource>(document)
  return {
    items: collection.items.map((item) => item.envelope).filter(Boolean),
    links: collection.links,
    page: collection.page
  }
}

export function submitPrompt(prompt: string): Promise<{ ok: boolean }> {
  return request('/api/prompt', {
    method: 'POST',
    body: resourceBody('prompt-submissions', { prompt })
  })
}

export function cancelRun(): Promise<{ ok: boolean }> {
  return request('/api/cancel', { method: 'POST', body: resourceBody('run-cancellations', {}) })
}

export function resumeSession(sessionId: string): Promise<{ ok: boolean }> {
  return request('/api/resume', {
    method: 'POST',
    body: resourceBody('session-resumes', { session_id: sessionId }, sessionId)
  })
}

export function answerPermission(
  id: string,
  decision: 'allow' | 'deny',
  scope: 'none' | 'session' | 'persistent' = 'none'
): Promise<{ ok: boolean }> {
  return request(`/api/permission/${encodeURIComponent(id)}`, {
    method: 'POST',
    body: resourceBody('permission-responses', { decision, scope }, id)
  })
}

export function answerAsk(id: string, answers: Record<string, string>): Promise<{ ok: boolean }> {
  return request(`/api/ask/${encodeURIComponent(id)}`, {
    method: 'POST',
    body: resourceBody('ask-responses', { answers }, id)
  })
}

export function loadArtifact(path: string): Promise<ArtifactResponse> {
  return request<ArtifactResponse>(`/api/artifact?path=${encodeURIComponent(path)}`)
}

export function subscribeEvents(onEvent: (event: EventEnvelope) => void): () => void {
  const source = new EventSource('/api/events')
  source.onmessage = (message) => {
    const document = JSON.parse(message.data) as JsonAPIDocument<{ envelope: EventEnvelope }>
    const attributes = unwrapJSONAPI<{ envelope: EventEnvelope }>(document)
    onEvent(attributes.envelope)
  }
  return () => source.close()
}
