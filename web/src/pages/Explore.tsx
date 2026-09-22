import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, ApiFailure, type Plot, type SiteUpdate, type Summary } from '../lib/api'
import { inr, sqft, PLOT_STATUS } from '../lib/format'
import { Spinner, ErrorNote, Field } from '../components/ui'

type PublicSociety = {
  id: string
  name: string
  slug: string
  builderName: string
  city?: string
  address?: string
  reraNumber?: string
  tagline?: string
  highlights?: { label: string; detail: string }[]
  amenities?: string[]
  brochureUrl?: string
  contactPhone?: string
  contactEmail?: string
}

const SECTORS = [
  { n: 1, label: 'Sector 01', range: '1 – 302', colour: '#E8913A' },
  { n: 2, label: 'Sector 02', range: '303 – 364', colour: '#4FA3DC' },
  { n: 3, label: 'Sector 03', range: '365 – 517', colour: '#57A55B' },
  { n: 4, label: 'Sector 04', range: '518 – 823', colour: '#8B7EC8' },
]

/**
 * The guest view. No account, no token — anyone with the link lands here.
 * It shows what is for sale and captures an enquiry; it shows nothing about
 * owners, dues, the fund or anyone's queries.
 */
export default function Explore() {
  const [society, setSociety] = useState<PublicSociety | null>(null)
  const [plots, setPlots] = useState<Plot[]>([])
  const [summary, setSummary] = useState<Summary | null>(null)
  const [updates, setUpdates] = useState<SiteUpdate[]>([])
  const [error, setError] = useState('')
  const [selected, setSelected] = useState<Plot | null>(null)
  const [onlyAvailable, setOnlyAvailable] = useState(true)
  const [sectorFilter, setSectorFilter] = useState<number | 'all'>('all')

  useEffect(() => {
    api
      .get<PublicSociety>('/api/public/society')
      .then(async (soc) => {
        setSociety(soc)
        const [p, s, u] = await Promise.all([
          api.get<{ plots: Plot[] }>(`/api/societies/${soc.id}/plots`),
          api.get<Summary>(`/api/societies/${soc.id}/summary`),
          api.get<{ updates: SiteUpdate[] }>(`/api/societies/${soc.id}/updates?limit=3`),
        ])
        setPlots(p.plots)
        setSummary(s)
        setUpdates(u.updates)
      })
      .catch(() => setError('Could not load the project right now.'))
  }, [])

  const visible = useMemo(
    () =>
      plots.filter((p) => {
        if (onlyAvailable && p.status !== 'available') return false
        if (sectorFilter !== 'all' && p.mapShape?.sector !== sectorFilter) return false
        return true
      }),
    [plots, onlyAvailable, sectorFilter],
  )

  const bounds = useMemo(() => {
    let maxX = 0
    let maxY = 0
    for (const p of plots) {
      for (const [x, y] of p.mapShape?.points ?? []) {
        if (x > maxX) maxX = x
        if (y > maxY) maxY = y
      }
    }
    return { w: maxX + 80, h: maxY + 80 }
  }, [plots])

  if (error) return <div className="mx-auto max-w-lg p-8"><ErrorNote message={error} /></div>
  if (!society || !summary) return <div className="grid min-h-screen place-items-center"><Spinner label="Loading the project" /></div>

  return (
    <div className="min-h-screen">
      {/* Hero */}
      <header className="relative overflow-hidden bg-olive-800 text-white">
        <div
          className="pointer-events-none absolute inset-0 opacity-45"
          style={{
            backgroundImage:
              'radial-gradient(900px 500px at 85% -10%, rgba(223,173,53,.55), transparent 60%),' +
              'radial-gradient(700px 600px at 5% 110%, rgba(79,163,220,.4), transparent 60%)',
          }}
        />
        <div className="relative mx-auto max-w-7xl px-5 py-6 sm:px-8">
          <nav className="flex items-center justify-between gap-4">
            <p className="text-[11px] font-semibold uppercase tracking-[0.28em] text-gold-300">
              {society.builderName}
            </p>
            <Link to="/login" className="btn-ghost border-white/30 bg-white/10 text-white hover:bg-white/20">
              Owner / builder sign in
            </Link>
          </nav>

          <div className="grid gap-10 py-12 lg:grid-cols-[1.1fr_1fr] lg:py-20">
            <div>
              <h1 className="font-display text-5xl font-semibold leading-[1.03] sm:text-6xl">
                {society.name}
              </h1>
              {society.tagline && (
                <p className="mt-4 max-w-lg font-display text-2xl text-gold-200">{society.tagline}</p>
              )}
              <p className="mt-4 max-w-xl text-olive-100">{society.address}</p>

              <div className="mt-8 flex flex-wrap gap-3">
                <a href="#layout" className="btn-gold">
                  See available plots
                </a>
                <a href="#enquire" className="btn-ghost border-white/30 bg-white/10 text-white hover:bg-white/20">
                  Talk to the site office
                </a>
              </div>

              {society.reraNumber && (
                <p className="mt-6 text-xs text-olive-200">
                  RERA <span className="font-mono">{society.reraNumber}</span> · NMRDA sanctioned
                </p>
              )}
            </div>

            <div className="grid grid-cols-2 gap-3 self-center">
              {(society.highlights ?? []).map((h) => (
                <div key={h.label} className="rounded-2xl bg-white/10 p-4 backdrop-blur">
                  <p className="font-display text-2xl font-semibold text-gold-300">{h.label}</p>
                  <p className="mt-0.5 text-xs leading-snug text-olive-100">{h.detail}</p>
                </div>
              ))}
            </div>
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-7xl space-y-14 px-5 py-14 sm:px-8">
        {/* Availability */}
        <section id="layout" className="scroll-mt-8">
          <div className="mb-5 flex flex-wrap items-end justify-between gap-3">
            <div>
              <h2 className="font-display text-3xl font-semibold text-olive-950">
                Live plot availability
              </h2>
              <p className="mt-1 text-sm text-olive-600">
                {summary.available} of {summary.total} plots are open right now. Tap any plot for its
                size and price.
              </p>
            </div>
            <div className="flex flex-wrap gap-2">
              <button
                onClick={() => setOnlyAvailable((v) => !v)}
                className={`chip border ${
                  onlyAvailable
                    ? 'border-emerald-600 bg-emerald-600 text-white'
                    : 'border-olive-200 bg-white text-olive-700'
                }`}
              >
                Available only
              </button>
              {SECTORS.map((s) => (
                <button
                  key={s.n}
                  onClick={() => setSectorFilter(sectorFilter === s.n ? 'all' : s.n)}
                  className={`chip border ${
                    sectorFilter === s.n ? 'text-white' : 'border-olive-200 bg-white text-olive-700'
                  }`}
                  style={
                    sectorFilter === s.n ? { background: s.colour, borderColor: s.colour } : undefined
                  }
                >
                  <span className="h-2 w-2 rounded-full" style={{ background: s.colour }} />
                  {s.label}
                </button>
              ))}
            </div>
          </div>

          <div className="card overflow-hidden p-0">
            <div className="max-h-[65vh] overflow-auto p-4">
              <svg viewBox={`0 0 ${bounds.w} ${bounds.h}`} className="h-auto w-full min-w-[900px]">
                {SECTORS.map((s) => {
                  const first = plots.find((p) => p.mapShape?.sector === s.n)
                  if (!first?.mapShape) return null
                  const y = Math.min(...first.mapShape.points.map(([, py]) => py))
                  return (
                    <text key={s.n} x={60} y={y - 14} fontSize="22" fontWeight="600" fill={s.colour}>
                      {s.label} · plots {s.range}
                    </text>
                  )
                })}
                {plots.map((p) => {
                  const shape = p.mapShape
                  if (!shape?.points?.length) return null
                  const dimmed = !visible.includes(p)
                  const meta = PLOT_STATUS[p.status]
                  const [x0, y0] = shape.points[0]!
                  const [x2, y2] = shape.points[2]!
                  return (
                    <g
                      key={p.id}
                      className={p.status === 'available' ? 'cursor-pointer' : ''}
                      opacity={dimmed ? 0.1 : 1}
                      onClick={() => p.status === 'available' && setSelected(p)}
                    >
                      <polygon
                        points={shape.points.map(([x, y]) => `${x},${y}`).join(' ')}
                        fill={meta?.dot ?? '#999'}
                        fillOpacity={p.status === 'available' ? 0.38 : 0.16}
                        stroke={meta?.dot ?? '#999'}
                        strokeWidth={selected?.id === p.id ? 3.5 : 1.2}
                      />
                      <text
                        x={(x0 + x2) / 2}
                        y={(y0 + y2) / 2 + 5}
                        textAnchor="middle"
                        fontSize="17"
                        fontWeight="600"
                        fill="#374327"
                      >
                        {p.plotNo}
                      </text>
                    </g>
                  )
                })}
              </svg>
            </div>
          </div>

          {selected && (
            <div className="card mt-4 flex flex-wrap items-center gap-4 p-5">
              <div className="grid h-14 w-14 shrink-0 place-items-center rounded-2xl bg-gradient-to-br from-emerald-500 to-emerald-700 font-display text-xl font-semibold text-white">
                {selected.plotNo}
              </div>
              <div className="min-w-0 flex-1">
                <p className="font-display text-xl font-semibold text-olive-950">
                  Plot {selected.plotNo} · {selected.phase}
                </p>
                <p className="text-sm text-olive-600">
                  {sqft(selected.areaSqft)} · {selected.facing} facing
                  {selected.isCorner ? ' · corner plot' : ''}
                </p>
              </div>
              {selected.price && (
                <p className="font-display text-2xl font-semibold text-olive-800">
                  {inr(selected.price, { compact: true })}
                </p>
              )}
              <a href="#enquire" className="btn-gold">
                Enquire about this plot
              </a>
            </div>
          )}
        </section>

        {/* Amenities */}
        {society.amenities && society.amenities.length > 0 && (
          <section>
            <h2 className="font-display text-3xl font-semibold text-olive-950">Amenities</h2>
            <p className="mt-1 text-sm text-olive-600">Planned across the sanctioned open spaces.</p>
            <div className="mt-5 flex flex-wrap gap-2">
              {society.amenities.map((a) => (
                <span
                  key={a}
                  className="chip border border-olive-200 bg-white px-3 py-1.5 text-olive-700"
                >
                  {a}
                </span>
              ))}
            </div>
          </section>
        )}

        {/* Recent progress — proof the project is moving */}
        {updates.length > 0 && (
          <section>
            <h2 className="font-display text-3xl font-semibold text-olive-950">Latest from site</h2>
            <div className="mt-5 grid gap-4 md:grid-cols-3">
              {updates.map((u) => (
                <article key={u.id} className="card p-5">
                  {u.phase && <span className="chip bg-olive-100 text-olive-700">{u.phase}</span>}
                  <h3 className="mt-2 font-display text-lg font-semibold text-olive-950">{u.title}</h3>
                  {u.body && <p className="mt-1.5 line-clamp-3 text-sm text-olive-600">{u.body}</p>}
                </article>
              ))}
            </div>
          </section>
        )}

        <EnquiryForm society={society} plot={selected} />
      </main>

      <footer className="border-t border-olive-200 bg-white/60 py-8">
        <div className="mx-auto max-w-7xl px-5 text-sm text-olive-600 sm:px-8">
          <p className="font-semibold text-olive-900">{society.builderName}</p>
          <p className="mt-1">{society.address}</p>
          <p className="mt-2 text-xs">
            RERA <span className="font-mono">{society.reraNumber}</span> · Plot numbers, sizes and
            sector ranges follow the NMRDA-sanctioned layout. Prices are indicative and confirmed by
            the site office.
          </p>
        </div>
      </footer>
    </div>
  )
}

function EnquiryForm({ society, plot }: { society: PublicSociety; plot: Plot | null }) {
  const [name, setName] = useState('')
  const [phone, setPhone] = useState('')
  const [email, setEmail] = useState('')
  const [budget, setBudget] = useState('')
  const [message, setMessage] = useState('')
  const [fields, setFields] = useState<Record<string, string>>({})
  const [error, setError] = useState('')
  const [done, setDone] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    setFields({})
    try {
      const res = await api.post<{ message: string }>('/api/public/enquiries', {
        societySlug: society.slug,
        ...(plot ? { plotId: plot.id, source: 'plot_detail' } : { source: 'layout_map' }),
        name,
        phone,
        email,
        budget,
        message,
      })
      setDone(res.message)
    } catch (err) {
      if (err instanceof ApiFailure) {
        setFields(err.detail.fields ?? {})
        if (!err.detail.fields) setError(err.detail.message)
      } else {
        setError('Could not send that. Please call the site office instead.')
      }
    } finally {
      setBusy(false)
    }
  }

  return (
    <section id="enquire" className="scroll-mt-8">
      <div className="card overflow-hidden">
        <div className="grid md:grid-cols-[1fr_1.2fr]">
          <div className="bg-gradient-to-br from-olive-700 to-olive-900 p-8 text-white">
            <h2 className="font-display text-3xl font-semibold">Talk to the site office</h2>
            <p className="mt-2 text-olive-100">
              Leave your number and someone from {society.builderName} will call you back — usually
              the same day.
            </p>
            {plot && (
              <p className="mt-5 rounded-xl bg-white/10 px-4 py-3 text-sm backdrop-blur">
                Asking about <strong>Plot {plot.plotNo}</strong> · {sqft(plot.areaSqft)}
              </p>
            )}
            <div className="mt-8 space-y-1 text-sm">
              {society.contactPhone && (
                <p>
                  <span className="text-olive-300">Phone</span>{' '}
                  <a className="font-semibold underline" href={`tel:${society.contactPhone}`}>
                    {society.contactPhone}
                  </a>
                </p>
              )}
              {society.contactEmail && (
                <p>
                  <span className="text-olive-300">Email</span>{' '}
                  <a className="font-semibold underline" href={`mailto:${society.contactEmail}`}>
                    {society.contactEmail}
                  </a>
                </p>
              )}
            </div>
          </div>

          <div className="p-8">
            {done ? (
              <div className="flex h-full flex-col items-center justify-center gap-3 text-center">
                <span className="grid h-14 w-14 place-items-center rounded-2xl bg-emerald-100 text-2xl">
                  ✓
                </span>
                <p className="font-display text-xl font-semibold text-olive-950">{done}</p>
                <p className="text-sm text-olive-600">
                  Keep browsing the layout in the meantime.
                </p>
              </div>
            ) : (
              <form onSubmit={submit} className="space-y-4">
                <div className="grid gap-4 sm:grid-cols-2">
                  <Field label="Your name" error={fields.name}>
                    <input
                      className="field"
                      required
                      value={name}
                      onChange={(e) => setName(e.target.value)}
                      placeholder="Sagar Waghmare"
                    />
                  </Field>
                  <Field label="Mobile" error={fields.phone}>
                    <input
                      className="field"
                      required
                      inputMode="tel"
                      value={phone}
                      onChange={(e) => setPhone(e.target.value)}
                      placeholder="98220 41190"
                    />
                  </Field>
                </div>

                <div className="grid gap-4 sm:grid-cols-2">
                  <Field label="Email (optional)">
                    <input
                      className="field"
                      type="email"
                      value={email}
                      onChange={(e) => setEmail(e.target.value)}
                      placeholder="you@example.com"
                    />
                  </Field>
                  <Field label="Budget (optional)">
                    <select className="field" value={budget} onChange={(e) => setBudget(e.target.value)}>
                      <option value="">Not sure yet</option>
                      <option>Under 30 L</option>
                      <option>30-40 L</option>
                      <option>40-50 L</option>
                      <option>50-60 L</option>
                      <option>60 L+</option>
                    </select>
                  </Field>
                </div>

                <Field label="Anything specific?" error={fields.message}>
                  <textarea
                    className="field min-h-[96px] resize-y"
                    value={message}
                    onChange={(e) => setMessage(e.target.value)}
                    placeholder="Looking for 1,500 sq ft plus, east facing. Can I visit this Sunday?"
                  />
                </Field>

                {error && <ErrorNote message={error} />}

                <button className="btn-primary w-full" disabled={busy}>
                  {busy ? 'Sending…' : 'Request a call back'}
                </button>
                <p className="text-center text-xs text-olive-500">
                  Your number goes to the site office only. No account needed.
                </p>
              </form>
            )}
          </div>
        </div>
      </div>
    </section>
  )
}
