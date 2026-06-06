import { describe, expect, it } from 'vitest'
import {
  activityRecords,
  eventRecord,
  filterEvents,
  jsonPretty,
  latestBlocking,
  mergeEventsBySequence,
  messageRecords,
  promptProjections
} from './workbench'
import type { EventEnvelope } from '../api/client'

const events: EventEnvelope[] = [
  {
    sequence: 1,
    received_at: '2026-06-06T00:00:00Z',
    type: 'model_request',
    data_type: 'query.ModelRequestEvent',
    data: { model: 'm1', unknown: 'kept' }
  },
  {
    sequence: 2,
    received_at: '2026-06-06T00:00:01Z',
    type: 'tool_call',
    data_type: 'query.ToolCallEvent',
    data: { call: { name: 'Bash', input: { cmd: 'date' } } }
  },
  {
    sequence: 3,
    received_at: '2026-06-06T00:00:02Z',
    type: 'permission_request',
    data_type: 'map[string]interface {}',
    data: { id: 'p1', tool: 'Bash', input: { cmd: 'date' } }
  }
]

describe('workbench projections', () => {
  it('filters visually without mutating source records', () => {
    const original = JSON.stringify(events)
    const filtered = filterEvents(events, 'tool', '')
    expect(filtered).toHaveLength(1)
    expect(filtered[0].data).toEqual({ call: { name: 'Bash', input: { cmd: 'date' } } })
    expect(JSON.stringify(events)).toBe(original)
  })

  it('searches full payload fields including unknown fields', () => {
    expect(filterEvents(events, 'all', 'kept')).toHaveLength(1)
  })

  it('keeps full event envelope in selected records', () => {
    expect(eventRecord(events[0]).source).toEqual(events[0])
  })

  it('merges backfilled and live events by sequence without duplicating source records', () => {
    const merged = mergeEventsBySequence([events[2], events[0]], [events[1], { ...events[2], data: { refreshed: true } }])
    expect(merged.map((event) => event.sequence)).toEqual([1, 2, 3])
    expect(merged[2].data).toEqual({ refreshed: true })
  })

  it('projects conversation messages without reducing source payloads', () => {
    const appState = {
      conversation: {
        messages: [
          {
            id: 'msg-1',
            role: 'user',
            content: [{ type: 'text', text: 'keep this source' }],
            unknown_message_field: 'visible'
          }
        ]
      }
    }

    const records = messageRecords(appState)
    expect(records[0].title).toBe('Message 1 User')
    expect(records[0].source).toEqual(appState.conversation.messages[0])
    expect(activityRecords(appState, events, 'text', 'unknown_message_field')[0].source).toEqual(appState.conversation.messages[0])
  })

  it('tracks the latest blocking prompt', () => {
    expect(latestBlocking(events)?.sequence).toBe(3)
  })

  it('derives prompt states from source request and resolution events', () => {
    const promptEvents: EventEnvelope[] = [
      ...events,
      {
        sequence: 4,
        received_at: '2026-06-06T00:00:03Z',
        type: 'permission_response',
        data_type: 'map[string]interface {}',
        data: { id: 'p1', body: { decision: 'deny' } }
      },
      {
        sequence: 5,
        received_at: '2026-06-06T00:00:04Z',
        type: 'ask_request',
        data_type: 'map[string]interface {}',
        data: {
          id: 'ask-1',
          request: {
            Questions: [
              {
                Question: 'Pick a branch',
                Header: 'Branch',
                Options: [{ Label: 'main', Description: 'Use main' }],
                MultiSelect: false
              }
            ]
          }
        }
      },
      {
        sequence: 6,
        received_at: '2026-06-06T00:00:05Z',
        type: 'ask_expired',
        data_type: 'map[string]interface {}',
        data: { id: 'ask-1', error: 'context canceled' }
      }
    ]

    const prompts = promptProjections(promptEvents)
    expect(prompts[0].status).toBe('denied')
    expect(prompts[0].resolutionEvent?.sequence).toBe(4)
    expect(prompts[1].status).toBe('expired')
    expect(prompts[1].fields[0]).toEqual({
      key: 'Pick a branch',
      label: 'Pick a branch',
      header: 'Branch',
      options: [{ label: 'main', description: 'Use main' }],
      multiSelect: false
    })
  })

  it('renders complete JSON for unknown fields', () => {
    expect(jsonPretty(events[0])).toContain('"unknown": "kept"')
  })
})
