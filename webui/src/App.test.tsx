import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { App } from './App'

class FakeEventSource {
  static instances: FakeEventSource[] = []
  onmessage: ((message: MessageEvent) => void) | null = null

  constructor(public url: string) {
    FakeEventSource.instances.push(this)
  }

  close = vi.fn()

  emit(data: unknown) {
    this.onmessage?.({ data: JSON.stringify(data) } as MessageEvent)
  }
}

describe('App', () => {
  let recentEventResources: unknown[]

  beforeEach(() => {
    recentEventResources = []
    FakeEventSource.instances = []
    vi.stubGlobal('EventSource', FakeEventSource)
    vi.stubGlobal(
      'fetch',
      vi.fn((path: string, init?: RequestInit) => {
        if (path === '/api/state') {
          return Promise.resolve(jsonAPIResource('runtime-states', 'active', {
            runtime: { workspace: '/repo', provider: 'lilac', model: 'glm', running: false },
            app_state: {
              conversation: {
                id: 'session-1',
                messages: [
                  {
                    id: 'msg-1',
                    role: 'user',
                    content: [{ type: 'text', text: 'inspect pagination source' }],
                    message_unknown_field: 'visible'
                  }
                ]
              },
              custom_state: { hidden: false },
              handoff_state: { goal: 'finish implementation', current_focus: 'web ui' },
              orchestration_artifacts: [
                {
                  path: '/tmp/pragma-handoff.md',
                  state_id: 'review',
                  event: 'complete',
                  direction: 'write',
                  unknown_field: 'visible'
                }
              ]
            }
          }))
        }
        if (path === '/api/sessions' || path === '/api/sessions?page%5Bnumber%5D=1&page%5Bsize%5D=1') {
          return Promise.resolve(jsonAPICollection([
            jsonAPIData('sessions', 'session-1', {
              model: 'glm',
              provider: 'lilac',
              work_dir: '/repo',
              turn_count: 2,
              cost_usd: 0.01,
              updated_at: '2026-06-06T00:00:00Z',
              session_unknown_field: 'visible'
            })
          ], { self: '/api/sessions?page%5Bnumber%5D=1&page%5Bsize%5D=1', next: '/api/sessions?page%5Bnumber%5D=2&page%5Bsize%5D=1' }, { number: 1, size: 1, total: 2, pages: 2, start: 0, end: 1 }))
        }
        if (path === '/api/sessions?page%5Bnumber%5D=2&page%5Bsize%5D=1') {
          return Promise.resolve(jsonAPICollection([
            jsonAPIData('sessions', 'session-2', {
              model: 'glm-next',
              provider: 'lilac',
              work_dir: '/repo/next',
              turn_count: 7,
              cost_usd: 0.02,
              updated_at: '2026-06-06T00:01:00Z',
              page_two_field: 'visible'
            })
          ], { self: '/api/sessions?page%5Bnumber%5D=2&page%5Bsize%5D=1', prev: '/api/sessions?page%5Bnumber%5D=1&page%5Bsize%5D=1' }, { number: 2, size: 1, total: 2, pages: 2, start: 1, end: 2 }))
        }
        if (path === '/api/sessions/session-1') {
          return Promise.resolve(jsonAPIResource('sessions', 'session-1', {
            conversation: {
              id: 'session-1',
              messages: [{ id: 'loaded-session-message', role: 'user', content: [{ type: 'text', text: 'full session source' }] }]
            },
            handoff_state: { current_focus: 'loaded from session store' },
            full_session_unknown_field: 'visible'
          }))
        }
        if (path === '/api/sessions/session-2') {
          return Promise.resolve(jsonAPIResource('sessions', 'session-2', {
            conversation: { id: 'session-2', messages: [] },
            full_page_two_field: 'visible'
          }))
        }
        if (String(path).startsWith('/api/completions')) {
          return Promise.resolve(jsonAPICollection([
            jsonAPIData('completions', '1', {
              label: '/doctor',
              detail: 'Run diagnostics',
              replacement: '/doctor ',
              completion_unknown_field: 'visible'
            }),
            jsonAPIData('completions', '2', {
              label: '/orchestrate',
              detail: 'Run orchestration',
              replacement: '/orchestrate '
            })
          ], { self: String(path) }, { number: 1, size: 50, total: 2, pages: 1, start: 0, end: 2 }))
        }
        if (String(path).startsWith('/api/events/recent')) {
          return Promise.resolve(jsonAPICollection(
            recentEventResources,
            { self: String(path) },
            { number: 1, size: 200, total: recentEventResources.length, pages: 1, start: 0, end: recentEventResources.length }
          ))
        }
        if (path === '/api/prompt' && init?.method === 'POST') {
          return Promise.resolve(jsonAPIResource('prompt-submissions', 'submit-1', { ok: true }))
        }
        if (path === '/api/resume' && init?.method === 'POST') {
          return Promise.resolve(jsonAPIResource('session-resumes', 'session-1', { ok: true }))
        }
        if (String(path).startsWith('/api/permission/')) {
          return Promise.resolve(jsonAPIResource('permission-responses', 'permit-1', { ok: true }))
        }
        if (String(path).startsWith('/api/ask/')) {
          return Promise.resolve(jsonAPIResource('ask-responses', 'ask-1', { ok: true }))
        }
        if (String(path).startsWith('/api/artifact')) {
          return Promise.resolve(jsonAPIResource('artifacts', 'artifact-1', {
            path: '/tmp/pragma-handoff.md',
            content: 'handoff content'
          }))
        }
        return Promise.resolve(jsonAPIError('not found', 404))
      })
    )
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders first screen workbench state and fixed composer', async () => {
    render(<App />)
    expect(await screen.findByText('Pragma')).toBeInTheDocument()
    expect(screen.getAllByText('lilac / glm').length).toBeGreaterThan(0)
    expect(screen.getByPlaceholderText('Start a message or run a slash command.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'workflow' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'handoffs' })).toBeInTheDocument()
  })

  it('shows session row metadata and loads full session JSON without resuming on row click', async () => {
    render(<App />)
    await screen.findByText('Recent sessions')
    const sessionList = screen.getByText('Recent sessions').closest('section')
    expect(sessionList).not.toBeNull()
    const sessionRow = within(sessionList as HTMLElement).getByRole('button', { name: /session-1.*2 turns/s })
    expect(sessionRow).toHaveTextContent('/repo')
    expect(sessionRow).toHaveTextContent('2 turns, $0.0100')
    expect(sessionRow).toHaveTextContent('2026')

    fireEvent.click(sessionRow)

    await waitFor(() => {
      expect(fetch).toHaveBeenCalledWith(
        '/api/sessions/session-1',
        expect.objectContaining({ headers: expect.objectContaining({ Accept: 'application/vnd.api+json' }) })
      )
    })
    expect(fetch).not.toHaveBeenCalledWith('/api/resume', expect.anything())
    expect(screen.getByText(/session_unknown_field/)).toBeInTheDocument()
    expect(screen.getByText(/full_session_unknown_field/)).toBeInTheDocument()
    expect(screen.getByText(/loaded-session-message/)).toBeInTheDocument()
  })

  it('resumes a selected session through the explicit inspector action', async () => {
    render(<App />)
    await screen.findByText('Recent sessions')
    const sessionList = screen.getByText('Recent sessions').closest('section')
    const sessionRow = within(sessionList as HTMLElement).getByRole('button', { name: /session-1.*2 turns/s })

    fireEvent.click(sessionRow)
    fireEvent.click(await screen.findByRole('button', { name: 'Resume session' }))

    await waitFor(() => {
      expect(fetch).toHaveBeenCalledWith('/api/resume', expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({
          data: {
            type: 'session-resumes',
            id: 'session-1',
            attributes: { session_id: 'session-1' }
          }
        })
      }))
    })
  })

  it('follows JSON:API pagination links for the session rail', async () => {
    render(<App />)
    expect(await screen.findByText('1-1 of 2')).toBeInTheDocument()
    const sessionList = screen.getByText('Recent sessions').closest('section')
    expect(within(sessionList as HTMLElement).getByRole('button', { name: /session-1.*2 turns/s })).toBeInTheDocument()

    fireEvent.click(screen.getByLabelText('Next sessions'))

    expect(await screen.findByText('session-2')).toBeInTheDocument()
    expect(screen.getByText('2-2 of 2')).toBeInTheDocument()
    await waitFor(() => {
      expect(fetch).toHaveBeenCalledWith(
        '/api/sessions?page%5Bnumber%5D=2&page%5Bsize%5D=1',
        expect.objectContaining({ headers: expect.objectContaining({ Accept: 'application/vnd.api+json' }) })
      )
    })

    fireEvent.click(screen.getByText('session-2'))
    expect(screen.getByText(/page_two_field/)).toBeInTheDocument()
    expect(await screen.findByText(/full_page_two_field/)).toBeInTheDocument()
  })

  it('submits composer text through the prompt endpoint', async () => {
    render(<App />)
    const composer = await screen.findByPlaceholderText('Start a message or run a slash command.')
    fireEvent.change(composer, { target: { value: '/doctor' } })
    fireEvent.click(screen.getByTitle('Send'))
    await waitFor(() => {
      expect(fetch).toHaveBeenCalledWith('/api/prompt', expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ data: { type: 'prompt-submissions', attributes: { prompt: '/doctor' } } })
      }))
    })
  })

  it('renders slash completions from the backend owner and applies replacements', async () => {
    render(<App />)
    const composer = await screen.findByPlaceholderText('Start a message or run a slash command.') as HTMLTextAreaElement
    fireEvent.change(composer, { target: { value: '/do' } })

    const option = await screen.findByRole('button', { name: /\/doctor.*Run diagnostics/s })
    expect(option).toBeInTheDocument()
    await waitFor(() => {
      expect(fetch).toHaveBeenCalledWith(
        '/api/completions?q=%2Fdo',
        expect.objectContaining({ headers: expect.objectContaining({ Accept: 'application/vnd.api+json' }) })
      )
    })

    fireEvent.click(option)
    expect(composer.value).toBe('/doctor ')
  })

  it('renders events with full JSON in the inspector', async () => {
    render(<App />)
    await screen.findByText('Pragma')
    act(() => {
      FakeEventSource.instances[0].emit(jsonAPIDocument(jsonAPIData('events', '4', {
        envelope: {
          sequence: 4,
          received_at: '2026-06-06T00:00:00Z',
          type: 'tool_call',
          data_type: 'query.ToolCallEvent',
          data: { call: { name: 'Bash' }, unknown_field: 'visible' }
        }
      })))
    })
    expect((await screen.findAllByText('#4 Bash')).length).toBeGreaterThan(0)
    expect(screen.getByText(/unknown_field/)).toBeInTheDocument()
  })

  it('backfills recent events from the JSON:API collection and dedupes SSE replay', async () => {
    const envelope = {
      sequence: 12,
      received_at: '2026-06-06T00:00:00Z',
      type: 'tool_call',
      data_type: 'query.ToolCallEvent',
      data: { call: { name: 'Read' }, backfill_unknown_field: 'visible' }
    }
    recentEventResources = [jsonAPIData('events', '12', { envelope })]

    render(<App />)

    expect(await screen.findByText('#12 Read')).toBeInTheDocument()
    await waitFor(() => {
      expect(fetch).toHaveBeenCalledWith(
        '/api/events/recent?page[number]=1&page[size]=200',
        expect.objectContaining({ headers: expect.objectContaining({ Accept: 'application/vnd.api+json' }) })
      )
    })

    act(() => {
      FakeEventSource.instances[0].emit(jsonAPIDocument(jsonAPIData('events', '12', { envelope })))
    })

    expect(await screen.findAllByRole('button', { name: /#12 Read/ })).toHaveLength(1)
    expect(screen.getByText(/backfill_unknown_field/)).toBeInTheDocument()
  })

  it('renders conversation messages as selectable source records in activity', async () => {
    render(<App />)
    await screen.findByText('Pragma')

    fireEvent.click(screen.getByRole('button', { name: 'activity' }))
    expect(await screen.findByRole('button', { name: /Message 1 User/ })).toBeInTheDocument()

    fireEvent.change(screen.getByPlaceholderText('Search source payloads'), { target: { value: 'pagination source' } })
    const messageRow = await screen.findByRole('button', { name: /Message 1 User/ })
    fireEvent.click(messageRow)

    const inspector = screen.getByLabelText('Inspector')
    expect(await within(inspector).findByText('Message 1 User')).toBeInTheDocument()
    expect(within(inspector).getByText(/message_unknown_field/)).toBeInTheDocument()
    expect(within(inspector).getByText(/inspect pagination source/)).toBeInTheDocument()
  })

  it('shows related identifiers and file text from the selected source object', async () => {
    render(<App />)
    await screen.findByText('Pragma')
    act(() => {
      FakeEventSource.instances[0].emit(jsonAPIDocument(jsonAPIData('events', '11', {
        envelope: {
          sequence: 11,
          received_at: '2026-06-06T00:00:00Z',
          type: 'slash_result',
          data_type: 'slash.Result',
          data: {
            id: 'slash-1',
            state_id: 'state-review',
            result: { tool_call_id: 'tool-call-7' },
            DisplayText: 'Available commands'
          }
        }
      })))
    })

    const inspector = screen.getByLabelText('Inspector')
    expect(await within(inspector).findByText('#11 Slash Result')).toBeInTheDocument()
    fireEvent.click(within(inspector).getByRole('button', { name: 'Related' }))
    expect(within(inspector).getByText('tool-call-7')).toBeInTheDocument()
    expect(within(inspector).getByText('state-review')).toBeInTheDocument()

    fireEvent.click(within(inspector).getByRole('button', { name: 'File/Text' }))
    expect(within(inspector).getByText('Available commands')).toBeInTheDocument()
  })

  it('shows permission controls before raw details', async () => {
    render(<App />)
    await screen.findByText('Pragma')
    act(() => {
      FakeEventSource.instances[0].emit(jsonAPIDocument(jsonAPIData('events', '5', {
        envelope: {
          sequence: 5,
          received_at: '2026-06-06T00:00:00Z',
          type: 'permission_request',
          data_type: 'map[string]interface {}',
          data: { id: 'permit-1', tool: 'Bash', input: { cmd: 'date' } }
        }
      })))
    })
    expect(await screen.findByRole('button', { name: 'Allow once' })).toBeInTheDocument()
    const panel = screen.getByLabelText('Permission request')
    const allowButton = within(panel).getByRole('button', { name: 'Allow once' })
    const fullJSONButton = within(panel).getByRole('button', { name: 'Full JSON' })
    expect(Boolean(allowButton.compareDocumentPosition(fullJSONButton) & Node.DOCUMENT_POSITION_FOLLOWING)).toBe(true)
    fireEvent.click(screen.getByRole('button', { name: 'Allow once' }))
    await waitFor(() => {
      expect(fetch).toHaveBeenCalledWith('/api/permission/permit-1', expect.objectContaining({ method: 'POST' }))
    })
  })

  it('submits session-scoped permission decisions through JSON:API attributes', async () => {
    render(<App />)
    await screen.findByText('Pragma')
    act(() => {
      FakeEventSource.instances[0].emit(jsonAPIDocument(jsonAPIData('events', '6', {
        envelope: {
          sequence: 6,
          received_at: '2026-06-06T00:00:00Z',
          type: 'permission_request',
          data_type: 'map[string]interface {}',
          data: { id: 'permit-session', tool: 'Edit', input: { file_path: 'README.md' } }
        }
      })))
    })
    fireEvent.click(await screen.findByRole('button', { name: 'Allow this session' }))
    await waitFor(() => {
      expect(fetch).toHaveBeenCalledWith('/api/permission/permit-session', expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({
          data: {
            type: 'permission-responses',
            id: 'permit-session',
            attributes: { decision: 'allow', scope: 'session' }
          }
        })
      }))
    })
  })

  it('shows denied state from permission response events and keeps request JSON visible', async () => {
    render(<App />)
    await screen.findByText('Pragma')
    act(() => {
      FakeEventSource.instances[0].emit(jsonAPIDocument(jsonAPIData('events', '7', {
        envelope: {
          sequence: 7,
          received_at: '2026-06-06T00:00:00Z',
          type: 'permission_request',
          data_type: 'map[string]interface {}',
          data: { id: 'permit-deny', tool: 'Bash', input: { cmd: 'rm -rf /tmp/x' }, reason: 'write command' }
        }
      })))
      FakeEventSource.instances[0].emit(jsonAPIDocument(jsonAPIData('events', '8', {
        envelope: {
          sequence: 8,
          received_at: '2026-06-06T00:00:01Z',
          type: 'permission_response',
          data_type: 'map[string]interface {}',
          data: { id: 'permit-deny', body: { decision: 'deny', scope: 'none' } }
        }
      })))
    })

    expect(await screen.findByText('denied')).toBeInTheDocument()
    expect(screen.getAllByText(/rm -rf/).length).toBeGreaterThan(0)
    expect(screen.getByRole('button', { name: 'Allow once' })).toBeDisabled()
  })

  it('submits structured ask answers by source question text', async () => {
    render(<App />)
    await screen.findByText('Pragma')
    act(() => {
      FakeEventSource.instances[0].emit(jsonAPIDocument(jsonAPIData('events', '9', {
        envelope: {
          sequence: 9,
          received_at: '2026-06-06T00:00:00Z',
          type: 'ask_request',
          data_type: 'map[string]interface {}',
          data: {
            id: 'ask-1',
            request: {
              Questions: [
                {
                  Question: 'Pick a branch',
                  Header: 'Branch',
                  Options: [{ Label: 'main', Description: 'Use main' }]
                }
              ]
            }
          }
        }
      })))
    })
    fireEvent.click(await screen.findByText('main'))
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Submit answer' })).not.toBeDisabled()
    })
    fireEvent.click(screen.getByRole('button', { name: 'Submit answer' }))
    await waitFor(() => {
      expect(fetch).toHaveBeenCalledWith('/api/ask/ask-1', expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({
          data: {
            type: 'ask-responses',
            id: 'ask-1',
            attributes: { answers: { 'Pick a branch': 'main' } }
          }
        })
      }))
    })
  })

  it('displays handoff state and fetches artifact content through the backend endpoint', async () => {
    render(<App />)
    await screen.findByText('Pragma')
    fireEvent.click(screen.getByRole('button', { name: 'handoffs' }))
    expect((await screen.findAllByText('Session handoff state')).length).toBeGreaterThan(0)
    expect(screen.getByText('Handoff files')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /pragma-handoff.md/ }))
    fireEvent.click(screen.getByRole('button', { name: 'Load artifact' }))
    await waitFor(() => {
      expect(fetch).toHaveBeenCalledWith(
        `/api/artifact?path=${encodeURIComponent('/tmp/pragma-handoff.md')}`,
        expect.objectContaining({ headers: expect.objectContaining({ Accept: 'application/vnd.api+json' }) })
      )
    })
    expect(await screen.findByText(/handoff content/)).toBeInTheDocument()
  })

  it('renders workflow and handoff rows from typed orchestration events with exact source JSON', async () => {
    render(<App />)
    await screen.findByText('Pragma')
    act(() => {
      FakeEventSource.instances[0].emit(jsonAPIDocument(jsonAPIData('events', '13', {
        envelope: {
          sequence: 13,
          received_at: '2026-06-06T00:00:00Z',
          type: 'orchestration_transition',
          data_type: 'query.OrchestrationTransitionEvent',
          data: {
            from_state: 'plan',
            to_state: 'review',
            transition_unknown_field: 'visible'
          }
        }
      })))
      FakeEventSource.instances[0].emit(jsonAPIDocument(jsonAPIData('events', '14', {
        envelope: {
          sequence: 14,
          received_at: '2026-06-06T00:00:01Z',
          type: 'orchestration_handoff',
          data_type: 'query.OrchestrationHandoffEvent',
          data: {
            state_id: 'review',
            path: '/tmp/handoff.md',
            handoff_unknown_field: 'visible'
          }
        }
      })))
    })

    fireEvent.click(screen.getByRole('button', { name: 'workflow' }))
    const workflowRow = await screen.findByRole('button', { name: /#13 Orchestration Transition/ })
    fireEvent.click(workflowRow)
    const workflowJSON = screen.getByText((content, element) => (
      element?.tagName.toLowerCase() === 'pre' &&
      content.includes('transition_unknown_field') &&
      content.includes('query.OrchestrationTransitionEvent')
    ))
    expect(workflowJSON).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'handoffs' }))
    const handoffRow = await screen.findByRole('button', { name: /#14 Orchestration Handoff/ })
    fireEvent.click(handoffRow)
    const handoffJSON = screen.getByText((content, element) => (
      element?.tagName.toLowerCase() === 'pre' &&
      content.includes('handoff_unknown_field') &&
      content.includes('query.OrchestrationHandoffEvent')
    ))
    expect(handoffJSON).toBeInTheDocument()
  })

  it('selects session_resumed events so the full loaded session payload is inspectable', async () => {
    render(<App />)
    await screen.findByText('Pragma')
    act(() => {
      FakeEventSource.instances[0].emit(jsonAPIDocument(jsonAPIData('events', '10', {
        envelope: {
          sequence: 10,
          received_at: '2026-06-06T00:00:00Z',
          type: 'session_resumed',
          data_type: 'session.Session',
          data: {
            conversation: {
              id: 'session-loaded',
              messages: [{ id: 'message-loaded', role: 'user', content: [{ type: 'text', text: 'full payload' }] }]
            },
            handoff_state: { current_focus: 'resume proof' },
            orchestration_artifacts: [{ path: '/tmp/resume-handoff.md' }],
            unknown_loaded_field: 'visible'
          }
        }
      })))
    })

    expect((await screen.findAllByText('#10 Session Resumed')).length).toBeGreaterThan(0)
    expect(screen.getByText(/unknown_loaded_field/)).toBeInTheDocument()
    expect(screen.getByText(/message-loaded/)).toBeInTheDocument()
  })
})

function jsonResponse(body: unknown, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    statusText: status === 200 ? 'OK' : 'Not Found',
    json: () => Promise.resolve(body),
    text: () => Promise.resolve(typeof body === 'string' ? body : JSON.stringify(body))
  } as Response
}

function jsonAPIData(type: string, id: string, attributes: unknown) {
  return { type, id, attributes }
}

function jsonAPIResource(type: string, id: string, attributes: unknown, status = 200) {
  return jsonResponse(jsonAPIDocument(jsonAPIData(type, id, attributes)), status)
}

function jsonAPIDocument(data: unknown) {
  return { jsonapi: { version: '1.1' }, data }
}

function jsonAPICollection(
  data: unknown[],
  links: Record<string, string> = { self: '/api/sessions?page%5Bnumber%5D=1&page%5Bsize%5D=50' },
  page: Record<string, number> = { number: 1, size: 50, total: data.length, pages: 1, start: 0, end: data.length },
  status = 200
) {
  return jsonResponse({
    jsonapi: { version: '1.1' },
    data,
    links,
    meta: { page }
  }, status)
}

function jsonAPIError(detail: string, status = 400) {
  return jsonResponse({ jsonapi: { version: '1.1' }, errors: [{ status: String(status), detail }] }, status)
}
