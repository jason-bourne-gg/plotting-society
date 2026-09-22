import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, type Query } from '../lib/api'
import { relative, shortDate } from '../lib/format'
import { Spinner, ErrorNote, Empty, QueryChip, SlaBadge } from '../components/ui'

export default function Queries() {
  const navigate = useNavigate()
  const [queries, setQueries] = useState<Query[] | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    api
      .get<{ queries: Query[] }>('/api/queries/mine')
      .then((r) => setQueries(r.queries))
      .catch(() => setError('Could not load your queries.'))
  }, [])

  if (error) return <ErrorNote message={error} />
  if (!queries) return <Spinner label="Loading your queries" />

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <p className="text-[11px] font-semibold uppercase tracking-[0.2em] text-gold-600">
            Tracked, with a deadline
          </p>
          <h1 className="mt-1 font-display text-4xl font-semibold text-olive-950">My queries</h1>
          <p className="mt-1 text-sm text-olive-600">
            Every question you have raised and where it stands.
          </p>
        </div>
        <button className="btn-gold" onClick={() => navigate('/queries/new')}>
          + Raise a query
        </button>
      </div>

      {queries.length === 0 ? (
        <Empty
          title="You have not raised anything yet"
          hint="Documents, dues, plot condition, roads, NOCs, resale or a site visit — raise it here and the clock starts."
          action={
            <button className="btn-primary" onClick={() => navigate('/queries/new')}>
              Raise your first query
            </button>
          }
        />
      ) : (
        <div className="space-y-3">
          {queries.map((q) => (
            <Link
              key={q.id}
              to={`/queries/${q.id}`}
              className="card block p-5 transition hover:shadow-lift"
            >
              <div className="flex flex-wrap items-center gap-2">
                <QueryChip status={q.status} />
                <SlaBadge breached={q.breached} due={q.slaDueAt} />
                {q.plotNo && (
                  <span className="chip bg-olive-100 text-olive-700">Plot {q.plotNo}</span>
                )}
                <span className="ml-auto text-xs text-olive-500">
                  Raised {shortDate(q.createdAt)}
                </span>
              </div>
              <h2 className="mt-2 font-semibold text-olive-950">{q.subject}</h2>
              <p className="mt-1 text-xs uppercase tracking-wide text-olive-500">
                {q.category.replace(/_/g, ' ')} · last activity {relative(q.updatedAt)}
              </p>
            </Link>
          ))}
        </div>
      )}
    </div>
  )
}
