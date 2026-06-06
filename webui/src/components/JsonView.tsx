import { ChevronDown, ChevronRight } from 'lucide-react'
import { useState } from 'react'
import type { JsonValue } from '../api/client'
import { jsonPretty } from '../features/workbench'

interface JsonViewProps {
  value: JsonValue | undefined
  defaultExpanded?: boolean
}

export function JsonView({ value, defaultExpanded = true }: JsonViewProps) {
  const [expanded, setExpanded] = useState(defaultExpanded)

  return (
    <section className="json-block">
      <button className="json-toggle" type="button" onClick={() => setExpanded((current) => !current)}>
        {expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        <span>Full JSON</span>
      </button>
      {expanded ? <pre>{jsonPretty(value)}</pre> : null}
    </section>
  )
}
