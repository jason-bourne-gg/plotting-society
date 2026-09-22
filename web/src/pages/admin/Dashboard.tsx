import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, type FundBalance, type Query, type Summary } from '../../lib/api'
import { useSociety } from '../../lib/useSociety'
import { inr, relative } from '../../lib/format'
import { Spinner, Stat, SectionTitle, QueryChip, SlaBadge } from '../../components/ui'

type Enquiry = { id: string; name: string; phone: string; plotNo?: string; status: string; createdAt: string }

export default function Dashboard() {
  const { society } = useSociety()
  const [summary, setSummary] = useState<Summary | null>(null)
  const [queries, setQueries] = useState<Query[]>([])
  const [leads, setLeads] = useState<Enquiry[]>([])
  const [counts, setCounts] = useState<Record<string, number>>({})
  const [balance, setBalance] = useState<FundBalance | null>(null)

  useEffect(() => {
    if (!society) return
    void Promise.all([
      api.get<Summary>(`/api/societies/${society.id}/summary`),
      api.get<{ queries: Query[] }>(`/api/societies/${society.id}/queries?limit=8`),
      api.get<{ enquiries: Enquiry[]; counts: Record<string, number> }>(
        `/api/societies/${society.id}/enquiries?limit=6`,
      ),
      api.get<{ balance: FundBalance }>(`/api/societies/${society.id}/fund?limit=1`),
    ]).then(([s, q, e, f]) => {
      setSummary(s)
      setQueries(q.queries)
      setLeads(e.enquiries)
      setCounts(e.counts)
      setBalance(f.balance)
    })
  }, [society])

  if (!summary || !balance) return <Spinner label="Loading dashboard" />

  const breached = queries.filter((q) => q.breached).length
  const open = queries.filter((q) => q.status === 'open' || q.status === 'in_progress').length

  return (
    <div className="space-y-8">
      <div>
        <p className="text-[11px] font-semibold uppercase tracking-[0.2em] text-gold-600">
          Builder portal
        </p>
        <h1 className="mt-1 font-display text-4xl font-semibold text-olive-950">{society?.name}</h1>
        <p className="mt-1 text-sm text-olive-600">
          What needs your attention today, in one screen.
        </p>
      </div>

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Stat
          label="Past SLA"
          value={String(breached)}
          accent={breached > 0 ? 'red' : 'emerald'}
          hint={breached > 0 ? 'Owners are waiting past the deadline' : 'Nothing overdue'}
        />
        <Stat label="Open queries" value={String(open)} hint="Open or in progress" />
        <Stat
          label="New leads"
          value={String(counts.new ?? 0)}
          accent="gold"
          hint={`${Object.values(counts).reduce((a, b) => a + b, 0)} enquiries total`}
        />
        <Stat
          label="Fund balance"
          value={inr(balance.closing, { compact: true })}
          hint={`${inr(balance.totalCredit, { compact: true })} collected`}
        />
      </div>

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Stat label="Total plots" value={String(summary.total)} />
        <Stat label="Sold" value={String(summary.sold)} />
        <Stat label="Booked" value={String(summary.booked)} accent="gold" />
        <Stat label="Available" value={String(summary.available)} accent="emerald" />
      </div>

      <section>
        <SectionTitle
          title="Query inbox"
          sub="Anything past its deadline sits at the top"
          action={
            <Link to="/admin/inbox" className="btn-ghost">
              Open inbox
            </Link>
          }
        />
        <div className="card divide-y divide-olive-100 overflow-hidden">
          {queries.slice(0, 6).map((q) => (
            <Link key={q.id} to={`/queries/${q.id}`} className="flex flex-wrap items-center gap-3 p-4 hover:bg-olive-50">
              <div className="min-w-0 flex-1">
                <p className="truncate font-semibold text-olive-950">{q.subject}</p>
                <p className="text-xs text-olive-500">
                  {q.raisedByName}
                  {q.plotNo && ` · Plot ${q.plotNo}`} · {relative(q.createdAt)}
                </p>
              </div>
              <QueryChip status={q.status} />
              <SlaBadge breached={q.breached} due={q.slaDueAt} />
            </Link>
          ))}
          {queries.length === 0 && <p className="p-6 text-sm text-olive-500">Nothing open. Rare and good.</p>}
        </div>
      </section>

      <section>
        <SectionTitle
          title="Recent enquiries"
          sub="Guests who left a number on the public layout"
          action={
            <Link to="/admin/leads" className="btn-ghost">
              All leads
            </Link>
          }
        />
        <div className="card divide-y divide-olive-100 overflow-hidden">
          {leads.map((l) => (
            <div key={l.id} className="flex flex-wrap items-center gap-3 p-4">
              <div className="min-w-0 flex-1">
                <p className="font-semibold text-olive-950">{l.name}</p>
                <p className="text-xs text-olive-500">
                  {l.plotNo ? `Plot ${l.plotNo} · ` : ''}
                  {relative(l.createdAt)}
                </p>
              </div>
              <a className="btn-ghost px-3 py-1.5 text-xs" href={`tel:${l.phone}`}>
                {l.phone}
              </a>
              <span className="chip bg-olive-100 text-olive-700">{l.status.replace(/_/g, ' ')}</span>
            </div>
          ))}
          {leads.length === 0 && <p className="p-6 text-sm text-olive-500">No enquiries yet.</p>}
        </div>
      </section>
    </div>
  )
}
