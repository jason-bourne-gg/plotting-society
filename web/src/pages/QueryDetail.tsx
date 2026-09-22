import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api, type Query, type QueryMessage } from '../lib/api'
import { useAuth } from '../lib/auth'
import { relative, shortDate } from '../lib/format'
import { Spinner, ErrorNote, QueryChip, SlaBadge } from '../components/ui'

const STAFF_STATUSES = ['open', 'in_progress', 'waiting_on_owner', 'resolved', 'closed']

export default function QueryDetail() {
  const { queryId } = useParams()
  const navigate = useNavigate()
  const { user, isStaff } = useAuth()

  const [query, setQuery] = useState<Query | null>(null)
  const [messages, setMessages] = useState<QueryMessage[]>([])
  const [reply, setReply] = useState('')
  const [internal, setInternal] = useState(false)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const load = useCallback(async () => {
    try {
      const res = await api.get<{ query: Query; messages: QueryMessage[] }>(
        `/api/queries/${queryId}`,
      )
      setQuery(res.query)
      setMessages(res.messages)
    } catch {
      setError('Could not load that query.')
    }
  }, [queryId])

  useEffect(() => {
    void load()
  }, [load])

  async function send(e: React.FormEvent) {
    e.preventDefault()
    if (!reply.trim()) return
    setBusy(true)
    try {
      await api.post(`/api/queries/${queryId}/messages`, {
        body: reply.trim(),
        ...(internal ? { isInternal: true } : {}),
      })
      setReply('')
      setInternal(false)
      await load()
    } catch {
      setError('Could not send that message.')
    } finally {
      setBusy(false)
    }
  }

  async function setStatus(status: string) {
    await api.patch(`/api/queries/${queryId}`, { status })
    await load()
  }

  if (error) return <ErrorNote message={error} />
  if (!query) return <Spinner label="Loading query" />

  return (
    <div className="mx-auto max-w-3xl space-y-5">
      <button className="text-sm font-semibold text-olive-600 hover:text-olive-800" onClick={() => navigate(-1)}>
        ← Back
      </button>

      <header className="card p-6">
        <div className="flex flex-wrap items-center gap-2">
          <QueryChip status={query.status} />
          <SlaBadge breached={query.breached} due={query.slaDueAt} />
          {query.plotNo && <span className="chip bg-olive-100 text-olive-700">Plot {query.plotNo}</span>}
          <span className="ml-auto text-xs text-olive-500">Raised {shortDate(query.createdAt)}</span>
        </div>

        <h1 className="mt-3 font-display text-2xl font-semibold text-olive-950">{query.subject}</h1>
        <p className="mt-1 text-xs uppercase tracking-wide text-olive-500">
          {query.category.replace(/_/g, ' ')} · by {query.raisedByName}
        </p>

        {query.breached && query.status !== 'resolved' && query.status !== 'closed' && (
          <p className="mt-3 rounded-xl border border-red-200 bg-red-50 px-3.5 py-2.5 text-sm text-red-800">
            This is past the response deadline the site office committed to.
          </p>
        )}

        {isStaff && (
          <div className="mt-4 flex flex-wrap gap-2 border-t border-olive-100 pt-4">
            {STAFF_STATUSES.map((s) => (
              <button
                key={s}
                onClick={() => void setStatus(s)}
                className={`chip border transition ${
                  query.status === s
                    ? 'border-olive-700 bg-olive-700 text-white'
                    : 'border-olive-200 bg-white text-olive-700 hover:border-olive-400'
                }`}
              >
                {s.replace(/_/g, ' ')}
              </button>
            ))}
          </div>
        )}
      </header>

      <div className="space-y-3">
        {messages.map((m) => {
          const own = m.authorId === user?.id
          return (
            <article
              key={m.id}
              className={`rounded-2xl border p-4 ${
                m.isInternal
                  ? 'border-dashed border-amber-300 bg-amber-50'
                  : own
                    ? 'ml-auto max-w-[85%] border-olive-700 bg-olive-700 text-white'
                    : 'mr-auto max-w-[85%] border-olive-200 bg-white'
              }`}
            >
              <div className="flex items-center gap-2 text-xs">
                <span className={`font-semibold ${own && !m.isInternal ? 'text-gold-300' : 'text-olive-600'}`}>
                  {m.authorName}
                </span>
                {m.isInternal && <span className="chip bg-amber-200 text-amber-900">Internal note</span>}
                <span className={own && !m.isInternal ? 'text-olive-200' : 'text-olive-400'}>
                  {relative(m.createdAt)}
                </span>
              </div>
              <p className="mt-1.5 whitespace-pre-wrap text-sm">{m.body}</p>
              {m.attachmentUrl && (
                <a
                  className="mt-2 inline-block text-xs font-semibold underline"
                  href={m.attachmentUrl}
                  target="_blank"
                  rel="noreferrer"
                >
                  Attachment
                </a>
              )}
            </article>
          )
        })}
      </div>

      <form onSubmit={send} className="card space-y-3 p-4">
        <textarea
          className="field min-h-[96px] resize-y"
          value={reply}
          onChange={(e) => setReply(e.target.value)}
          placeholder={isStaff ? 'Reply to the owner…' : 'Add to this thread…'}
        />
        <div className="flex flex-wrap items-center gap-3">
          <button className="btn-primary" disabled={busy || !reply.trim()}>
            {busy ? 'Sending…' : 'Send'}
          </button>
          {isStaff && (
            <label className="flex items-center gap-2 text-sm text-olive-700">
              <input
                type="checkbox"
                checked={internal}
                onChange={(e) => setInternal(e.target.checked)}
                className="h-4 w-4 rounded border-olive-300"
              />
              Internal note — the owner will not see this
            </label>
          )}
        </div>
      </form>
    </div>
  )
}
