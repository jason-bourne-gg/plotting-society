import { useCallback, useEffect, useState } from 'react'
import { api } from '../../lib/api'
import { useSociety } from '../../lib/useSociety'
import { relative } from '../../lib/format'
import { Spinner, Empty, SectionTitle, Stat } from '../../components/ui'

type Enquiry = {
  id: string
  plotNo?: string
  name: string
  phone: string
  email?: string
  message?: string
  budget?: string
  source: string
  status: string
  notes?: string
  createdAt: string
}

const STATUSES = ['new', 'contacted', 'visit_scheduled', 'converted', 'lost']

const STATUS_STYLE: Record<string, string> = {
  new: 'bg-gold-100 text-gold-800',
  contacted: 'bg-sky-100 text-sky-800',
  visit_scheduled: 'bg-violet-100 text-violet-800',
  converted: 'bg-emerald-100 text-emerald-800',
  lost: 'bg-slate-200 text-slate-700',
}

export default function Leads() {
  const { society } = useSociety()
  const [leads, setLeads] = useState<Enquiry[] | null>(null)
  const [counts, setCounts] = useState<Record<string, number>>({})
  const [filter, setFilter] = useState('')

  const load = useCallback(async () => {
    if (!society) return
    const r = await api.get<{ enquiries: Enquiry[]; counts: Record<string, number> }>(
      `/api/societies/${society.id}/enquiries${filter ? `?status=${filter}` : ''}`,
    )
    setLeads(r.enquiries)
    setCounts(r.counts)
  }, [society, filter])

  useEffect(() => {
    void load()
  }, [load])

  async function move(id: string, status: string) {
    await api.patch(`/api/enquiries/${id}`, { status })
    await load()
  }

  const total = Object.values(counts).reduce((a, b) => a + b, 0)

  return (
    <div className="space-y-5">
      <SectionTitle
        title="Leads"
        sub="Enquiries from guests browsing the public layout — no account needed on their side"
      />

      <div className="grid gap-4 sm:grid-cols-4">
        <Stat label="Total" value={String(total)} />
        <Stat label="New" value={String(counts.new ?? 0)} accent="gold" hint="Not yet called" />
        <Stat label="Visits booked" value={String(counts.visit_scheduled ?? 0)} />
        <Stat label="Converted" value={String(counts.converted ?? 0)} accent="emerald" />
      </div>

      <div className="flex flex-wrap gap-2">
        <button
          onClick={() => setFilter('')}
          className={`chip border ${filter === '' ? 'border-olive-700 bg-olive-700 text-white' : 'border-olive-200 bg-white text-olive-700'}`}
        >
          All
        </button>
        {STATUSES.map((s) => (
          <button
            key={s}
            onClick={() => setFilter(s)}
            className={`chip border ${filter === s ? 'border-olive-700 bg-olive-700 text-white' : 'border-olive-200 bg-white text-olive-700'}`}
          >
            {s.replace(/_/g, ' ')} {counts[s] ? `(${counts[s]})` : ''}
          </button>
        ))}
      </div>

      {!leads ? (
        <Spinner />
      ) : leads.length === 0 ? (
        <Empty
          title="No enquiries yet"
          hint="Share the public layout link and every enquiry lands here with the plot they were looking at."
        />
      ) : (
        <div className="space-y-3">
          {leads.map((l) => (
            <article key={l.id} className="card p-5">
              <div className="flex flex-wrap items-start gap-3">
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <h3 className="font-display text-lg font-semibold text-olive-950">{l.name}</h3>
                    <span className={`chip ${STATUS_STYLE[l.status] ?? 'bg-olive-100 text-olive-700'}`}>
                      {l.status.replace(/_/g, ' ')}
                    </span>
                    {l.plotNo && (
                      <span className="chip bg-olive-100 text-olive-700">Plot {l.plotNo}</span>
                    )}
                    {l.budget && <span className="chip bg-gold-100 text-gold-800">{l.budget}</span>}
                  </div>
                  {l.message && <p className="mt-2 text-sm text-olive-700">{l.message}</p>}
                  <p className="mt-2 text-xs text-olive-500">
                    via {l.source.replace(/_/g, ' ')} · {relative(l.createdAt)}
                    {l.email && ` · ${l.email}`}
                  </p>
                </div>

                <div className="flex flex-col items-end gap-2">
                  <a className="btn-gold px-3 py-2 text-sm" href={`tel:${l.phone}`}>
                    📞 {l.phone}
                  </a>
                  <a
                    className="btn-ghost px-3 py-1.5 text-xs"
                    href={`https://wa.me/${l.phone.replace(/\D/g, '')}`}
                    target="_blank"
                    rel="noreferrer"
                  >
                    WhatsApp
                  </a>
                </div>
              </div>

              <div className="mt-4 flex flex-wrap gap-2 border-t border-olive-100 pt-3">
                {STATUSES.map((s) => (
                  <button
                    key={s}
                    onClick={() => void move(l.id, s)}
                    className={`chip border text-xs transition ${
                      l.status === s
                        ? 'border-olive-700 bg-olive-700 text-white'
                        : 'border-olive-200 bg-white text-olive-600 hover:border-olive-400'
                    }`}
                  >
                    {s.replace(/_/g, ' ')}
                  </button>
                ))}
              </div>
            </article>
          ))}
        </div>
      )}
    </div>
  )
}
