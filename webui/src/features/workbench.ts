import type { EventEnvelope, JsonValue, RawRecord, RuntimeState, SessionSummary } from '../api/client'

export type WorkbenchTab = 'overview' | 'workflow' | 'handoffs' | 'activity' | 'state'
export type EventFilter = 'all' | 'model' | 'text' | 'tool' | 'permission' | 'ask' | 'orchestration' | 'error'

export interface SelectableRecord {
  id: string
  kind: 'run' | 'event' | 'message' | 'session' | 'state' | 'permission' | 'ask' | 'workflow' | 'handoff' | 'artifact'
  title: string
  subtitle?: string
  source: JsonValue
  event?: EventEnvelope
}

export type PromptKind = 'permission' | 'ask'
export type PromptStatus = 'pending' | 'answered' | 'denied' | 'expired'

export interface AskField {
  key: string
  label: string
  header?: string
  options: Array<{ label: string; description?: string }>
  multiSelect: boolean
}

export interface PromptProjection {
  kind: PromptKind
  id: string
  status: PromptStatus
  title: string
  subtitle: string
  requestEvent: EventEnvelope
  resolutionEvent?: EventEnvelope
  request: RawRecord
  fields: AskField[]
  decision?: string
}

const filterGroups: Record<EventFilter, string[]> = {
  all: [],
  model: ['model_request', 'model_response'],
  text: ['text', 'thinking', 'user_message', 'prompt_accepted', 'slash_result'],
  tool: ['tool_call', 'tool_result'],
  permission: ['permission_request', 'permission_response', 'permission_expired'],
  ask: ['ask_request', 'ask_response', 'ask_expired'],
  orchestration: ['orchestration_', 'workflow_snapshot'],
  error: ['run_error', 'compaction_failed']
}

export function filterEvents(events: EventEnvelope[], filter: EventFilter, query: string): EventEnvelope[] {
  const trimmed = query.trim().toLowerCase()
  return events.filter((event) => {
    const types = filterGroups[filter]
    const typeMatch =
      filter === 'all' ||
      types.some((type) => (type.endsWith('_') ? event.type.startsWith(type) : event.type === type))
    if (!typeMatch) return false
    if (!trimmed) return true
    return JSON.stringify(event).toLowerCase().includes(trimmed)
  })
}

export function mergeEventsBySequence(current: EventEnvelope[], incoming: EventEnvelope[]): EventEnvelope[] {
  const bySequence = new Map<number, EventEnvelope>()
  for (const event of current) bySequence.set(event.sequence, event)
  for (const event of incoming) bySequence.set(event.sequence, event)
  return Array.from(bySequence.values()).sort((left, right) => left.sequence - right.sequence)
}

export function activityRecords(
  appState: RawRecord | undefined,
  events: EventEnvelope[],
  filter: EventFilter,
  query: string
): SelectableRecord[] {
  const records = [...messageRecords(appState), ...events.map(eventRecord)]
  const trimmed = query.trim().toLowerCase()
  return records.filter((record) => {
    const typeMatch =
      filter === 'all' ||
      (record.kind === 'message' && filter === 'text') ||
      (record.event ? eventMatchesFilter(record.event, filter) : false)
    if (!typeMatch) return false
    if (!trimmed) return true
    return JSON.stringify(record.source).toLowerCase().includes(trimmed)
  })
}

export function eventTitle(event: EventEnvelope): string {
  switch (event.type) {
    case 'prompt_accepted':
      return 'Prompt accepted'
    case 'permission_request':
      return 'Permission requested'
    case 'permission_response':
      return 'Permission answered'
    case 'permission_expired':
      return 'Permission expired'
    case 'ask_request':
      return 'Ask requested'
    case 'ask_response':
      return 'Ask answered'
    case 'ask_expired':
      return 'Ask expired'
    case 'model_request':
      return 'Model request'
    case 'model_response':
      return 'Model response'
    case 'tool_call':
      return toolName(event.data) ?? 'Tool call'
    case 'tool_result':
      return toolCallID(event.data) ?? 'Tool result'
    case 'workflow_snapshot':
      return 'Workflow snapshot'
    case 'run_error':
      return 'Runtime error'
    default:
      return titleFromType(event.type)
  }
}

export function eventSubtitle(event: EventEnvelope): string {
  const details = [event.data_type, new Date(event.received_at).toLocaleTimeString()].filter(Boolean)
  return details.join(' | ')
}

export function eventRecord(event: EventEnvelope): SelectableRecord {
  return {
    id: `event:${event.sequence}`,
    kind: event.type.startsWith('permission_') ? 'permission' : event.type.startsWith('ask_') ? 'ask' : 'event',
    title: `#${event.sequence} ${eventTitle(event)}`,
    subtitle: eventSubtitle(event),
    source: event,
    event
  }
}

export function messageRecords(appState: RawRecord | undefined): SelectableRecord[] {
  const conversation = objectValue(appState?.conversation)
  const messages = arrayValue(conversation?.messages) ?? []
  return messages.flatMap((raw, index) => {
    const message = objectValue(raw)
    if (!message) return []
    const id = stringValue(message.id) || `${index + 1}`
    const role = stringValue(message.role) || 'message'
    return [{
      id: `message:${id}`,
      kind: 'message' as const,
      title: `Message ${index + 1} ${titleFromType(role)}`,
      subtitle: messageSubtitle(message),
      source: message
    }]
  })
}

export function sessionRecord(session: SessionSummary, fullSession?: RawRecord): SelectableRecord {
  return {
    id: `session:${session.id}`,
    kind: 'session',
    title: session.id,
    subtitle: [session.provider, session.model, session.work_dir].filter(Boolean).join(' | '),
    source: fullSession ? { summary: session, session: fullSession } : session
  }
}

export function runRecord(runtime: RuntimeState, appState: RawRecord | undefined): SelectableRecord {
  return {
    id: 'run:active',
    kind: 'run',
    title: 'Active run',
    subtitle: [runtime.provider, runtime.model, runtime.workspace].filter(Boolean).join(' | '),
    source: { runtime, app_state: appState ?? {} }
  }
}

export function jsonPretty(value: JsonValue | undefined): string {
  return JSON.stringify(value ?? null, null, 2)
}

export function latestBlocking(events: EventEnvelope[]): EventEnvelope | undefined {
  return latestPromptProjection(events, 'pending')?.requestEvent
}

export function latestPromptProjection(
  events: EventEnvelope[],
  statusFilter?: PromptStatus
): PromptProjection | undefined {
  const prompts = promptProjections(events)
  for (let i = prompts.length - 1; i >= 0; i -= 1) {
    if (!statusFilter || prompts[i].status === statusFilter) return prompts[i]
  }
  return undefined
}

export function promptProjections(events: EventEnvelope[]): PromptProjection[] {
  const resolutions = events.filter(isPromptResolution)
  return events
    .filter((event) => event.type === 'permission_request' || event.type === 'ask_request')
    .map((event) => {
      const data = objectValue(event.data) ?? {}
      const id = stringValue(data.id)
      const kind: PromptKind = event.type === 'permission_request' ? 'permission' : 'ask'
      const resolution = resolutions.find((candidate) => promptResolutionMatches(candidate, kind, id, event.sequence))
      const resolutionData = objectValue(resolution?.data)
      const body = objectValue(resolutionData?.body)
      const decision = kind === 'permission' ? stringValue(body?.decision) : undefined
      const status = promptStatus(kind, resolution, decision)
      const request = kind === 'ask' ? objectValue(data.request) ?? data : data
      return {
        kind,
        id,
        status,
        title: kind === 'permission' ? 'Permission request' : 'Ask request',
        subtitle: promptSubtitle(kind, data, status, decision),
        requestEvent: event,
        resolutionEvent: resolution,
        request,
        fields: kind === 'ask' ? askFields(request) : [],
        decision
      }
    })
}

export function workflowEvents(events: EventEnvelope[]): EventEnvelope[] {
  return events.filter((event) => event.type.startsWith('orchestration_') || event.type === 'workflow_snapshot')
}

export function handoffEvents(events: EventEnvelope[]): EventEnvelope[] {
  return events.filter((event) => event.type === 'orchestration_handoff')
}

export function handoffSurfaceRecords(appState: RawRecord | undefined, events: EventEnvelope[]): SelectableRecord[] {
  const records: SelectableRecord[] = []
  const handoffState = objectValue(appState?.handoff_state)
  if (handoffState && Object.keys(handoffState).length > 0) {
    records.push({
      id: 'handoff:state',
      kind: 'handoff',
      title: 'Session handoff state',
      subtitle: 'app_state.handoff_state',
      source: handoffState
    })
  }
  const artifacts = arrayValue(appState?.orchestration_artifacts) ?? []
  for (const raw of artifacts) {
    const artifact = objectValue(raw)
    const artifactPath = stringValue(artifact?.path)
    if (!artifact || !artifactPath) continue
    records.push({
      id: `artifact:${artifactPath}`,
      kind: 'artifact',
      title: artifactPath,
      subtitle: [stringValue(artifact.state_id), stringValue(artifact.event), stringValue(artifact.direction)].filter(Boolean).join(' | '),
      source: artifact
    })
  }
  return [...records, ...handoffEvents(events).map(eventRecord)]
}

export function stateSessionID(appState: RawRecord | undefined): string {
  const conversation = objectValue(appState?.conversation)
  const id = conversation?.id
  return typeof id === 'string' ? id : ''
}

export function titleFromType(type: string): string {
  return type
    .split('_')
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}

function eventMatchesFilter(event: EventEnvelope, filter: EventFilter): boolean {
  const types = filterGroups[filter]
  return filter === 'all' || types.some((type) => (type.endsWith('_') ? event.type.startsWith(type) : event.type === type))
}

function messageSubtitle(message: RawRecord): string {
  const role = stringValue(message.role)
  const id = stringValue(message.id)
  return [role, id].filter(Boolean).join(' | ')
}

export function objectValue(value: JsonValue | undefined): RawRecord | undefined {
  return value && typeof value === 'object' && !Array.isArray(value) ? (value as RawRecord) : undefined
}

export function stringValue(value: JsonValue | undefined): string {
  return typeof value === 'string' ? value : ''
}

function isPromptResolution(event: EventEnvelope): boolean {
  return event.type === 'permission_response' || event.type === 'permission_expired' || event.type === 'ask_response' || event.type === 'ask_expired'
}

function promptResolutionMatches(event: EventEnvelope, kind: PromptKind, id: string, requestSequence: number): boolean {
  if (!id || event.sequence <= requestSequence) return false
  if (kind === 'permission' && !event.type.startsWith('permission_')) return false
  if (kind === 'ask' && !event.type.startsWith('ask_')) return false
  return promptID(event) === id
}

function promptID(event: EventEnvelope): string {
  const data = objectValue(event.data)
  return stringValue(data?.id)
}

function promptStatus(kind: PromptKind, resolution: EventEnvelope | undefined, decision: string | undefined): PromptStatus {
  if (!resolution) return 'pending'
  if (resolution.type.endsWith('_expired')) return 'expired'
  if (kind === 'permission' && decision === 'deny') return 'denied'
  return 'answered'
}

function promptSubtitle(kind: PromptKind, data: RawRecord, status: PromptStatus, decision: string | undefined): string {
  const state = status === 'denied' ? 'denied' : status
  if (kind === 'permission') {
    const tool = stringValue(data.tool) || 'tool'
    const reason = stringValue(data.reason)
    return [tool, decision || state, reason].filter(Boolean).join(' | ')
  }
  const request = objectValue(data.request)
  const question = firstAskQuestion(request)
  return [question || 'answer required', state].filter(Boolean).join(' | ')
}

function firstAskQuestion(request: RawRecord | undefined): string {
  if (!request) return ''
  const legacy = stringValue(request.Question) || stringValue(request.question)
  if (legacy) return legacy
  return askFields(request)[0]?.label ?? ''
}

function askFields(request: RawRecord | undefined): AskField[] {
  if (!request) return [{ key: 'answer', label: 'Answer', options: [], multiSelect: false }]
  const questions = arrayValue(request.Questions) ?? arrayValue(request.questions)
  if (!questions || questions.length === 0) {
    const question = stringValue(request.Question) || stringValue(request.question) || 'Answer'
    return [{ key: question, label: question, options: [], multiSelect: false }]
  }
  return questions.map((raw, index) => {
    const question = objectValue(raw) ?? {}
    const label = stringValue(question.Question) || stringValue(question.question) || `Question ${index + 1}`
    return {
      key: label,
      label,
      header: stringValue(question.Header) || stringValue(question.header),
      options: optionValues(question.Options) ?? optionValues(question.options) ?? [],
      multiSelect: Boolean(question.MultiSelect ?? question.multiSelect)
    }
  })
}

function optionValues(value: JsonValue | undefined): Array<{ label: string; description?: string }> | undefined {
  const options = arrayValue(value)
  if (!options) return undefined
  return options
    .map((raw) => {
      const option = objectValue(raw) ?? {}
      const label = stringValue(option.Label) || stringValue(option.label)
      if (!label) return undefined
      const description = stringValue(option.Description) || stringValue(option.description)
      return { label, ...(description ? { description } : {}) }
    })
    .filter((option): option is { label: string; description?: string } => Boolean(option))
}

function arrayValue(value: JsonValue | undefined): JsonValue[] | undefined {
  return Array.isArray(value) ? value : undefined
}

function toolName(value: JsonValue): string | undefined {
  const data = objectValue(value)
  const call = objectValue(data?.call)
  const name = call?.name
  return typeof name === 'string' ? name : undefined
}

function toolCallID(value: JsonValue): string | undefined {
  const data = objectValue(value)
  const result = objectValue(data?.result)
  const id = result?.tool_call_id ?? result?.id
  return typeof id === 'string' ? id : undefined
}
