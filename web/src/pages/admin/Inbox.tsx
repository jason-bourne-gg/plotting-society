import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, type Query } from '../../lib/api'
import { useSociety } from '../../lib/useSociety'
import { relative, shortDate } from '../../lib/format'
import { Spinner, Empty, QueryChip, SlaBadge, SectionTitle } from '../../components/ui'

const FILTERS = [
  { key: '', label: 'Everything' },
  { key: 'open', label: 'Open' },
  { key: 'in_progress', label: 'In progress' },
  { key: 'waiting_on_owner', label: 'Waiting on owner' },
  { key: 'resolved', label: 'Resolved' },
]

export default function Inbox() {
  const { society } = useSociety()
  const [queries, setQueries] = useState<Query[] | null>(null)
  const [filter, setFilter] = useState('')

  useEffect(() => {
    if (!society) return
    setQueries(null)
    api
      .get<{ queries: Query[] }>(
        `/api/societies/${society.id}/queries${filter ? `?status=${filter}` : ''}`,
      )
      .then((r) => setQueries(r.queries))
  }, [society, filter])

  return (
    <div className="space-y-5">
      <SectionTitle
        title="Query inbox"
        sub="Owners' questions, breached deadlines first"
      />

      <div className="flex flex-wrap gap-2">
        {FILTERS.map((f) => (
          <button
            key={f.key}
            onClick={() => setFilter(f.key)}
            className={`chip border ${
              filter === f.key
                ? 'border-olive-700 bg-olive-700 text-white'
                : 'border-olive-200 bg-white text-olive-700'
            }`}
          >
            {f.label}
          </button>
        ))}
      </div>

      {!queries ? (
        <Spinner />
      ) : queries.length === 0 ? (
        <Empty title="Nothing here" hint="No queries match that filter." />
      ) : (
        <div className="space-y-3">
          {queries.map((q) => (
            <Link
              key={q.id}
              to={`/queries/${q.id}`}
              className={`card block p-5 transition hover:shadow-lift ${
                q.breached ? 'border-red-300 bg-red-50/60' : ''
              }`}
            >
              <div className="flex flex-wrap items-center gap-2">
                <QueryChip status={q.status} />
                <SlaBadge breached={q.breached} due={q.slaDueAt} />
                {q.plotNo && <span className="chip bg-olive-100 text-olive-700">Plot {q.plotNo}</span>}
                <span className="ml-auto text-xs text-olive-500">{shortDate(q.createdAt)}</span>
              </div>
              <h3 className="mt-2 font-semibold text-olive-950">{q.subject}</h3>
              <p className="mt-1 text-xs uppercase tracking-wide text-olive-500">
                {q.category.replace(/_/g, ' ')} · {q.raisedByName} · last activity{' '}
                {relative(q.updatedAt)}
              </p>
            </Link>
          ))}
        </div>
      )}
    </div>
  )
}
