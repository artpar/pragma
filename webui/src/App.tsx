import { ChevronLeft, ChevronRight, Search, Square } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import {
  cancelRun,
  loadRecentEvents,
  loadSession,
  loadSessions,
  loadState,
  resumeSession,
  submitPrompt,
  subscribeEvents,
  type CollectionResult,
  type EventEnvelope,
  type SessionSummary,
  type StateResponse
} from './api/client'
import { Composer } from './components/Composer'
import { Inspector } from './components/Inspector'
import { JsonView } from './components/JsonView'
import { PromptPanel } from './components/PromptPanel'
import {
  activityRecords,
  eventRecord,
  eventSubtitle,
  eventTitle,
  handoffSurfaceRecords,
  latestPromptProjection,
  mergeEventsBySequence,
  runRecord,
  sessionRecord,
  stateSessionID,
  workflowEvents,
  type EventFilter,
  type SelectableRecord,
  type WorkbenchTab
} from './features/workbench'

const tabs: WorkbenchTab[] = ['overview', 'workflow', 'handoffs', 'activity', 'state']
const filters: EventFilter[] = ['all', 'model', 'text', 'tool', 'permission', 'ask', 'orchestration', 'error']

export function App() {
  const [state, setState] = useState<StateResponse>({})
  const [sessions, setSessions] = useState<SessionSummary[]>([])
  const [sessionPage, setSessionPage] = useState<CollectionResult<SessionSummary> | undefined>()
  const [events, setEvents] = useState<EventEnvelope[]>([])
  const [selected, setSelected] = useState<SelectableRecord | undefined>()
  const [tab, setTab] = useState<WorkbenchTab>('overview')
  const [filter, setFilter] = useState<EventFilter>('all')
  const [query, setQuery] = useState('')
  const [prompt, setPrompt] = useState('')
  const [submitError, setSubmitError] = useState('')
  const [drawerOpen, setDrawerOpen] = useState(false)
  const sessionPagePath = useRef('/api/sessions')

  useEffect(() => {
    void refreshState()
    void refreshEvents()
    const unsubscribe = subscribeEvents((event) => {
      setEvents((current) => mergeEventsBySequence(current, [event]))
      setSelected((current) => (event.type === 'session_resumed' ? eventRecord(event) : current ?? eventRecord(event)))
      if (event.type === 'run_idle' || event.type === 'session_resumed') void refreshState()
    })
    return unsubscribe
  }, [])

  async function refreshState(nextSessionPagePath = sessionPagePath.current) {
    const [nextState, nextSessions] = await Promise.all([loadState(), loadSessions(nextSessionPagePath)])
    sessionPagePath.current = nextSessions.links.self || nextSessionPagePath
    setState(nextState)
    setSessions(nextSessions.items)
    setSessionPage(nextSessions)
  }

  async function refreshEvents() {
    try {
      const recent = await loadRecentEvents()
      setEvents((current) => mergeEventsBySequence(current, recent.items))
    } catch (error) {
      setSubmitError(error instanceof Error ? error.message : String(error))
    }
  }

  async function navigateSessions(path: string | undefined) {
    if (!path) return
    setSubmitError('')
    try {
      await refreshState(path)
    } catch (error) {
      setSubmitError(error instanceof Error ? error.message : String(error))
    }
  }

  const runtime = state.runtime ?? {}
  const appState = state.app_state
  const activeSessionID = stateSessionID(appState)
  const promptState = latestPromptProjection(events)
  const visibleActivity = useMemo(() => activityRecords(appState, events, filter, query), [appState, events, filter, query])
  const workflows = workflowEvents(events)
  const handoffRecords = handoffSurfaceRecords(appState, events)
  const activeRecord = selected ?? runRecord(runtime, appState)

  function select(record: SelectableRecord) {
    setSelected(record)
    setDrawerOpen(true)
  }

  async function submit(text: string) {
    setSubmitError('')
    try {
      await submitPrompt(text)
      setPrompt('')
      void refreshState()
    } catch (error) {
      setSubmitError(error instanceof Error ? error.message : String(error))
    }
  }

  async function resume(id: string) {
    setSubmitError('')
    try {
      await resumeSession(id)
      void refreshState()
    } catch (error) {
      setSubmitError(error instanceof Error ? error.message : String(error))
    }
  }

  async function selectSession(session: SessionSummary) {
    select(sessionRecord(session))
    try {
      const fullSession = await loadSession(session.id)
      setSelected(sessionRecord(session, fullSession))
    } catch (error) {
      setSubmitError(error instanceof Error ? error.message : String(error))
    }
  }

  async function cancel() {
    setSubmitError('')
    try {
      await cancelRun()
    } catch (error) {
      setSubmitError(error instanceof Error ? error.message : String(error))
    }
  }

  return (
    <div className="app-shell">
      <aside className="session-rail" aria-label="Sessions">
        <header className="brand-strip">
          <strong>Pragma</strong>
          <span>{runtime.running ? 'running' : 'idle'}</span>
        </header>
        <section className="run-card">
          <button type="button" onClick={() => select(runRecord(runtime, appState))}>
            <span>Active run</span>
            <strong>{activeSessionID || 'new session'}</strong>
          </button>
          <dl>
            <div>
              <dt>Provider</dt>
              <dd>{runtime.provider || 'unknown'}</dd>
            </div>
            <div>
              <dt>Model</dt>
              <dd>{runtime.model || 'unknown'}</dd>
            </div>
            <div>
              <dt>Workspace</dt>
              <dd>{runtime.workspace || 'unknown'}</dd>
            </div>
          </dl>
        </section>
        <section className="sessions-list">
          <h2>Recent sessions</h2>
          <div className="rail-scroll">
            {sessions.length === 0 ? <p className="empty-text">No saved sessions.</p> : null}
            {sessions.map((session) => (
              <button
                key={session.id}
                type="button"
                className={session.id === activeSessionID ? 'session-row active' : 'session-row'}
                onClick={() => void selectSession(session)}
              >
                <strong>{session.id}</strong>
                <span>{[session.provider, session.model].filter(Boolean).join(' / ') || 'model unknown'}</span>
                <span>{session.work_dir || 'workdir unknown'}</span>
                <small>
                  {session.turn_count ?? 0} turns, ${Number(session.cost_usd ?? 0).toFixed(4)}
                  {session.updated_at ? `, ${new Date(session.updated_at).toLocaleString()}` : ''}
                </small>
              </button>
            ))}
          </div>
          <SessionPageBar page={sessionPage} onNavigate={navigateSessions} />
        </section>
      </aside>

      <main className="workbench">
        <header className="top-strip">
          <div>
            <span>{runtime.workspace || 'workspace unknown'}</span>
            <strong>{[runtime.provider, runtime.model].filter(Boolean).join(' / ') || 'model unknown'}</strong>
          </div>
          {runtime.running ? (
            <button type="button" onClick={cancel} title="Cancel run">
              <Square size={16} />
            </button>
          ) : null}
        </header>

        <div className="prompt-slot">{promptState ? <PromptPanel prompt={promptState} /> : null}</div>

        <nav className="tabs" aria-label="Workbench tabs">
          {tabs.map((name) => (
            <button key={name} className={tab === name ? 'active' : ''} type="button" onClick={() => setTab(name)}>
              {name}
            </button>
          ))}
        </nav>

        <section className="tab-panel">
          {tab === 'overview' ? (
            <Overview runtimeRecord={runRecord(runtime, appState)} events={events} onSelect={select} />
          ) : null}
          {tab === 'workflow' ? (
            <RecordList
              empty="No workflow events yet."
              records={workflows.map(eventRecord)}
              onSelect={select}
            />
          ) : null}
          {tab === 'handoffs' ? (
            <HandoffView records={handoffRecords} onSelect={select} />
          ) : null}
          {tab === 'activity' ? (
            <Activity
              records={visibleActivity}
              filter={filter}
              query={query}
              onFilter={setFilter}
              onQuery={setQuery}
              onSelect={select}
            />
          ) : null}
          {tab === 'state' ? <JsonView value={{ runtime, app_state: appState ?? {} }} /> : null}
        </section>

        <Composer value={prompt} running={Boolean(runtime.running)} error={submitError} onChange={setPrompt} onSubmit={submit} />
      </main>

      <Inspector activeRecord={activeRecord} open={drawerOpen} onClose={() => setDrawerOpen(false)} onResume={resume} />
    </div>
  )
}

function SessionPageBar({
  page,
  onNavigate
}: {
  page: CollectionResult<SessionSummary> | undefined
  onNavigate: (path: string | undefined) => void
}) {
  const total = page?.page?.total ?? page?.items.length ?? 0
  const start = total > 0 ? (page?.page?.start ?? 0) + 1 : 0
  const end = page?.page?.end ?? page?.items.length ?? 0
  return (
    <footer className="session-pagebar" aria-label="Session pages">
      <span>
        {start}-{end} of {total}
      </span>
      <div>
        <button type="button" onClick={() => onNavigate(page?.links.prev)} disabled={!page?.links.prev} title="Previous sessions" aria-label="Previous sessions">
          <ChevronLeft size={15} />
        </button>
        <button type="button" onClick={() => onNavigate(page?.links.next)} disabled={!page?.links.next} title="Next sessions" aria-label="Next sessions">
          <ChevronRight size={15} />
        </button>
      </div>
    </footer>
  )
}

function Overview({
  runtimeRecord,
  events,
  onSelect
}: {
  runtimeRecord: SelectableRecord
  events: EventEnvelope[]
  onSelect: (record: SelectableRecord) => void
}) {
  const recent = events.slice(-6).reverse()
  return (
    <div className="overview-grid">
      <button className="summary-line" type="button" onClick={() => onSelect(runtimeRecord)}>
        <span>Runtime state</span>
        <strong>{runtimeRecord.subtitle || 'source available'}</strong>
      </button>
      <section className="recent-activity">
        <h2>Recent activity</h2>
        {recent.length === 0 ? <p className="empty-text">Start a message or run a slash command.</p> : null}
        {recent.map((event) => (
          <EventRow key={event.sequence} event={event} onSelect={onSelect} />
        ))}
      </section>
    </div>
  )
}

function Activity({
  records,
  filter,
  query,
  onFilter,
  onQuery,
  onSelect
}: {
  records: SelectableRecord[]
  filter: EventFilter
  query: string
  onFilter: (filter: EventFilter) => void
  onQuery: (query: string) => void
  onSelect: (record: SelectableRecord) => void
}) {
  return (
    <div className="activity-view">
      <div className="activity-tools">
        <label>
          <Search size={15} />
          <input value={query} onChange={(event) => onQuery(event.target.value)} placeholder="Search source payloads" />
        </label>
        <div className="filter-row">
          {filters.map((name) => (
            <button key={name} className={filter === name ? 'active' : ''} type="button" onClick={() => onFilter(name)}>
              {name}
            </button>
          ))}
        </div>
      </div>
      {records.length === 0 ? <p className="empty-text">No matching records.</p> : null}
      <div className="event-list">
        {records.map((record) => (
          <button key={record.id} className="event-row" type="button" onClick={() => onSelect(record)}>
            <strong>{record.title}</strong>
            <span>{record.subtitle}</span>
          </button>
        ))}
      </div>
    </div>
  )
}

function RecordList({
  records,
  empty,
  onSelect
}: {
  records: SelectableRecord[]
  empty: string
  onSelect: (record: SelectableRecord) => void
}) {
  if (records.length === 0) return <p className="empty-text">{empty}</p>
  return (
    <div className="event-list">
      {records.map((record) => (
        <button key={record.id} className="event-row" type="button" onClick={() => onSelect(record)}>
          <strong>{record.title}</strong>
          <span>{record.subtitle}</span>
        </button>
      ))}
    </div>
  )
}

function HandoffView({
  records,
  onSelect
}: {
  records: SelectableRecord[]
  onSelect: (record: SelectableRecord) => void
}) {
  if (records.length === 0) return <p className="empty-text">No handoffs yet.</p>
  const stateRecords = records.filter((record) => record.kind === 'handoff' && record.id === 'handoff:state')
  const artifacts = records.filter((record) => record.kind === 'artifact')
  const events = records.filter((record) => record.kind !== 'artifact' && record.id !== 'handoff:state')
  return (
    <div className="handoff-view">
      <RecordSection title="Session handoff state" records={stateRecords} empty="No session handoff state." onSelect={onSelect} />
      <RecordSection title="Handoff files" records={artifacts} empty="No recorded handoff files." onSelect={onSelect} />
      <RecordSection title="Handoff events" records={events} empty="No handoff events yet." onSelect={onSelect} />
    </div>
  )
}

function RecordSection({
  title,
  records,
  empty,
  onSelect
}: {
  title: string
  records: SelectableRecord[]
  empty: string
  onSelect: (record: SelectableRecord) => void
}) {
  return (
    <section className="record-section">
      <h2>{title}</h2>
      {records.length === 0 ? <p className="empty-text">{empty}</p> : null}
      {records.map((record) => (
        <button key={record.id} className="event-row" type="button" onClick={() => onSelect(record)}>
          <strong>{record.title}</strong>
          <span>{record.subtitle}</span>
        </button>
      ))}
    </section>
  )
}

function EventRow({
  event,
  onSelect
}: {
  event: EventEnvelope
  onSelect: (record: SelectableRecord) => void
}) {
  return (
    <button className="event-row" type="button" onClick={() => onSelect(eventRecord(event))}>
      <strong>
        #{event.sequence} {eventTitle(event)}
      </strong>
      <span>{eventSubtitle(event)}</span>
    </button>
  )
}
