import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, type Plot, type Summary } from '../lib/api'
import { useSociety } from '../lib/useSociety'
import { sqft, PLOT_STATUS } from '../lib/format'
import { Spinner, Stat, ErrorNote } from '../components/ui'
import PlotMap, { MapLegend, SECTORS } from '../components/PlotMap'

export default function MapPage() {
  const { society } = useSociety()
  const navigate = useNavigate()

  const [plots, setPlots] = useState<Plot[] | null>(null)
  const [summary, setSummary] = useState<Summary | null>(null)
  const [error, setError] = useState('')
  const [sector, setSector] = useState<number | null>(1)
  const [statusFilter, setStatusFilter] = useState('all')
  const [search, setSearch] = useState('')

  useEffect(() => {
    if (!society) return
    Promise.all([
      api.get<{ plots: Plot[] }>(`/api/societies/${society.id}/plots`),
      api.get<Summary>(`/api/societies/${society.id}/summary`),
    ])
      .then(([p, s]) => {
        setPlots(p.plots)
        setSummary(s)
        // Open on the sector the viewer owns a plot in — the reason most
        // owners come here at all.
        const mine = p.plots.find(x => x.isMine)
        if (mine?.mapShape?.sector) setSector(mine.mapShape.sector)
      })
      .catch(() => setError('Could not load the layout.'))
  }, [society])

  // Per-sector counts drive the selector, so a sector can be chosen on what is
  // left in it rather than on its number.
  const bySector = useMemo(() => {
    const out = new Map<number, { total: number; available: number; mine: boolean }>()
    for (const p of plots ?? []) {
      const n = p.mapShape?.sector
      if (!n) continue
      const e = out.get(n) ?? { total: 0, available: 0, mine: false }
      e.total++
      if (p.status === 'available') e.available++
      if (p.isMine) e.mine = true
      out.set(n, e)
    }
    return out
  }, [plots])

  // A search that finds a plot in another sector should go to it, not come up
  // empty because the wrong sector happens to be open.
  useEffect(() => {
    const q = search.trim()
    if (!q || !plots) return
    const hit = plots.find(p => p.plotNo === q)
    if (hit?.mapShape?.sector && hit.mapShape.sector !== sector) {
      setSector(hit.mapShape.sector)
    }
  }, [search, plots, sector])

  if (error) return <ErrorNote message={error} />
  if (!plots || !summary) return <Spinner label="Loading the layout" />

  const mine = plots.find(p => p.isMine)
  const shownCount = plots.filter(
    p =>
      (sector === null || p.mapShape?.sector === sector) &&
      (statusFilter === 'all' || p.status === statusFilter) &&
      (!search || p.plotNo.includes(search.trim())),
  ).length

  return (
    <div className="space-y-6">
      <div>
        <p className="text-[11px] font-semibold uppercase tracking-[0.2em] text-gold-600">
          {society?.city} · NMRDA sanctioned
        </p>
        <h1 className="mt-1 font-display text-4xl font-semibold text-olive-950">Layout map</h1>
        <p className="mt-1 max-w-2xl text-sm text-olive-600">
          Live status for all {summary.total} plots. Pick a sector, then tap a plot for its size,
          facing and price.
        </p>
      </div>

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Stat label="Total plots" value={String(summary.total)} hint="4 sectors · 58 acres" />
        <Stat label="Available" value={String(summary.available)} accent="emerald" hint="Open for booking" />
        <Stat label="Booked" value={String(summary.booked)} accent="gold" hint="Advance received" />
        <Stat label="Sold" value={String(summary.sold)} hint="Registered to owners" />
      </div>

      {mine && (
        <button
          onClick={() => navigate('/my-plot')}
          className="card flex w-full items-center gap-4 p-4 text-left transition hover:shadow-lift"
        >
          <span className="grid h-12 w-12 shrink-0 place-items-center rounded-xl bg-gradient-to-br from-gold-400 to-gold-600 font-display text-lg font-semibold text-olive-950">
            {mine.plotNo}
          </span>
          <span className="min-w-0">
            <span className="block font-semibold text-olive-950">Your plot · {mine.phase}</span>
            <span className="block truncate text-sm text-olive-600">
              {sqft(mine.areaSqft)} · {mine.facing} facing{mine.isCorner ? ' · corner' : ''}
            </span>
          </span>
          <span className="ml-auto text-olive-400">→</span>
        </button>
      )}

      {/* Sector selector. Four cards beat a row of filter chips: each one
          carries the count that decides which sector you want to look at. */}
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
        {SECTORS.map(s => {
          const c = bySector.get(s.n)
          const active = sector === s.n
          return (
            <button
              key={s.n}
              onClick={() => setSector(s.n)}
              className={`card overflow-hidden p-0 text-left transition ${
                active ? 'shadow-lift ring-2 ring-offset-2 ring-offset-cream' : 'hover:shadow-lift'
              }`}
              style={active ? { boxShadow: `0 0 0 2px ${s.colour}` } : undefined}
            >
              <div className="h-1.5" style={{ background: s.colour }} />
              <div className="p-3.5">
                <div className="flex items-center justify-between gap-2">
                  <span className="font-semibold text-olive-950">{s.label}</span>
                  {c?.mine && <span className="chip bg-gold-100 text-gold-800">yours</span>}
                </div>
                <p className="mt-0.5 text-[11px] uppercase tracking-wide text-olive-500">
                  plots {s.range}
                </p>
                <p className="mt-2 text-sm">
                  <span className="font-display text-xl font-semibold text-emerald-700">
                    {c?.available ?? 0}
                  </span>
                  <span className="text-olive-500"> of {c?.total ?? 0} available</span>
                </p>
              </div>
            </button>
          )
        })}

        <button
          onClick={() => setSector(null)}
          className={`card p-3.5 text-left transition hover:shadow-lift ${
            sector === null ? 'shadow-lift ring-2 ring-olive-700' : ''
          }`}
        >
          <span className="font-semibold text-olive-950">Whole layout</span>
          <p className="mt-0.5 text-[11px] uppercase tracking-wide text-olive-500">all 4 sectors</p>
          <p className="mt-2 text-sm text-olive-500">Zoom to read plot numbers</p>
        </button>
      </div>

      {/* Filters, kept to one line */}
      <div className="card flex flex-wrap items-center gap-2 p-3">
        <input
          className="field max-w-[170px]"
          placeholder="Go to plot no."
          inputMode="numeric"
          value={search}
          onChange={e => setSearch(e.target.value)}
        />
        <span className="mx-1 hidden h-6 w-px bg-olive-200 sm:block" />
        {['all', ...Object.keys(PLOT_STATUS)].map(key => (
          <button
            key={key}
            onClick={() => setStatusFilter(key)}
            className={`chip border ${
              statusFilter === key
                ? 'border-olive-700 bg-olive-700 text-white'
                : 'border-olive-200 bg-white text-olive-700'
            }`}
          >
            {key === 'all' ? (
              'Any status'
            ) : (
              <>
                <span className="h-2 w-2 rounded-full" style={{ background: PLOT_STATUS[key]!.dot }} />
                {PLOT_STATUS[key]!.label}
              </>
            )}
          </button>
        ))}
        <span className="ml-auto text-xs font-semibold text-olive-500">{shownCount} shown</span>
      </div>

      <PlotMap
        plots={plots}
        sector={sector}
        statusFilter={statusFilter}
        search={search}
        onPick={p => navigate(`/plots/${p.id}`)}
      />

      <div className="flex flex-wrap items-center justify-between gap-3">
        <MapLegend />
        <p className="text-xs text-olive-500">
          Owner names and contact details are never shown on the map.
        </p>
      </div>
    </div>
  )
}
