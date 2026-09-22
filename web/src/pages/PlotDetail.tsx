import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api, type Plot, type PlotDetail as Detail } from '../lib/api'
import { useSociety } from '../lib/useSociety'
import { inr, safeUrl, sqft, shortDate } from '../lib/format'
import { Spinner, ErrorNote, StatusChip, SectionTitle, Empty } from '../components/ui'

/** `mine` resolves the signed-in owner's plot instead of taking one from the URL. */
export default function PlotDetailPage({ mine = false }: { mine?: boolean }) {
  const { plotId } = useParams()
  const { society } = useSociety()
  const navigate = useNavigate()

  const [detail, setDetail] = useState<Detail | null>(null)
  const [error, setError] = useState('')
  const [noPlot, setNoPlot] = useState(false)

  useEffect(() => {
    let alive = true
    ;(async () => {
      try {
        let id = plotId
        if (mine) {
          if (!society) return
          const res = await api.get<{ plots: Plot[] }>(`/api/societies/${society.id}/plots`)
          const own = res.plots.find((p) => p.isMine)
          if (!own) {
            if (alive) setNoPlot(true)
            return
          }
          id = own.id
        }
        const d = await api.get<Detail>(`/api/plots/${id}`)
        if (alive) setDetail(d)
      } catch {
        if (alive) setError('Could not load that plot.')
      }
    })()
    return () => {
      alive = false
    }
  }, [plotId, mine, society])

  if (error) return <ErrorNote message={error} />
  if (noPlot)
    return (
      <Empty
        title="No plot is linked to your account yet"
        hint="The site office links your plot when registration is complete. Raise a query if you think this is wrong."
        action={
          <button className="btn-primary" onClick={() => navigate('/queries/new')}>
            Raise a query
          </button>
        }
      />
    )
  if (!detail) return <Spinner label="Loading plot" />

  const outstanding = (detail.dues ?? []).reduce(
    (sum, d) => sum + Math.max(0, d.amountDue - d.amountPaid),
    0,
  )

  return (
    <div className="space-y-6">
      <button className="text-sm font-semibold text-olive-600 hover:text-olive-800" onClick={() => navigate(-1)}>
        ← Back
      </button>

      <div className="card overflow-hidden">
        <div className="relative bg-gradient-to-br from-olive-700 via-olive-800 to-olive-950 p-6 text-white sm:p-8">
          <div
            className="pointer-events-none absolute inset-0 opacity-40"
            style={{
              backgroundImage:
                'radial-gradient(500px 260px at 90% 0%, rgba(223,173,53,.45), transparent 60%)',
            }}
          />
          <div className="relative flex flex-wrap items-start justify-between gap-4">
            <div>
              <p className="text-[11px] font-semibold uppercase tracking-[0.2em] text-gold-300">
                {detail.phase} {detail.isMine && '· your plot'}
              </p>
              <h1 className="mt-1 font-display text-5xl font-semibold">Plot {detail.plotNo}</h1>
              <p className="mt-2 text-olive-100">
                {sqft(detail.areaSqft)} · {detail.facing} facing
                {detail.isCorner ? ' · corner plot' : ''}
              </p>
            </div>
            <div className="text-right">
              <StatusChip status={detail.status} />
              {detail.price && detail.status === 'available' && (
                <p className="mt-2 font-display text-2xl font-semibold text-gold-300">
                  {inr(detail.price, { compact: true })}
                </p>
              )}
            </div>
          </div>
        </div>

        <div className="grid gap-px bg-olive-200 sm:grid-cols-3">
          {[
            ['Area', sqft(detail.areaSqft)],
            ['Facing', detail.facing || '—'],
            ['Sector', detail.phase || '—'],
          ].map(([label, value]) => (
            <div key={label} className="bg-white p-4">
              <p className="text-xs font-semibold uppercase tracking-wider text-olive-500">{label}</p>
              <p className="mt-1 font-semibold text-olive-950">{value}</p>
            </div>
          ))}
        </div>
      </div>

      {detail.dues && detail.dues.length > 0 && (
        <section>
          <SectionTitle
            title="Maintenance"
            sub={
              outstanding > 0
                ? `${inr(outstanding)} outstanding`
                : 'All paid up — nothing outstanding'
            }
          />
          <div className="card divide-y divide-olive-100 overflow-hidden">
            {detail.dues.map((d) => {
              const due = Math.max(0, d.amountDue - d.amountPaid)
              return (
                <div key={d.id} className="flex flex-wrap items-center gap-3 p-4">
                  <div className="min-w-0 flex-1">
                    <p className="font-semibold text-olive-950">{d.periodLabel}</p>
                    {/* Show the working. A bare total invites a query; the
                        arithmetic answers it before it is raised. */}
                    {d.ratePerSqft != null && d.areaSqft != null && (
                      <p className="text-xs font-medium text-olive-600">
                        {sqft(d.areaSqft)} × ₹{d.ratePerSqft.toFixed(2)}/sq ft
                      </p>
                    )}
                    <p className="text-xs text-olive-500">
                      {d.paidOn ? `Paid ${shortDate(d.paidOn)}` : `Due ${shortDate(d.dueDate)}`}
                    </p>
                  </div>
                  <p className="font-semibold text-olive-800">{inr(d.amountDue)}</p>
                  {due > 0 ? (
                    <span className="chip bg-amber-100 text-amber-800">{inr(due)} due</span>
                  ) : (
                    <span className="chip bg-emerald-100 text-emerald-800">Paid</span>
                  )}
                  {safeUrl(d.receiptUrl) && (
                    <a
                      className="btn-ghost px-3 py-1.5 text-xs"
                      href={safeUrl(d.receiptUrl)!}
                      target="_blank"
                      rel="noreferrer noopener"
                    >
                      Receipt
                    </a>
                  )}
                </div>
              )
            })}
          </div>
        </section>
      )}

      {detail.documents && detail.documents.length > 0 && (
        <section>
          <SectionTitle title="Documents" sub="Everything the site office has filed against this plot" />
          <div className="grid gap-3 sm:grid-cols-2">
            {detail.documents.map((doc) => (
              <a
                key={doc.id}
                href={safeUrl(doc.url) ?? '#'}
                target="_blank"
                rel="noreferrer noopener"
                className="card flex items-center gap-3 p-4 transition hover:shadow-lift"
              >
                <span className="grid h-10 w-10 place-items-center rounded-xl bg-olive-100">📄</span>
                <span className="min-w-0">
                  <span className="block truncate font-semibold text-olive-950">{doc.title}</span>
                  <span className="block text-xs uppercase tracking-wide text-olive-500">
                    {doc.docType.replace(/_/g, ' ')}
                  </span>
                </span>
              </a>
            ))}
          </div>
        </section>
      )}

      {detail.isMine && (
        <div className="card flex flex-wrap items-center gap-3 p-5">
          <div className="flex-1">
            <p className="font-semibold text-olive-950">Something not right with this plot?</p>
            <p className="text-sm text-olive-600">
              Raise it here and you will see the site office&apos;s deadline to respond.
            </p>
          </div>
          <button className="btn-gold" onClick={() => navigate('/queries/new')}>
            Raise a query
          </button>
        </div>
      )}
    </div>
  )
}
