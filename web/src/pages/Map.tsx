import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, type Plot, type Summary } from '../lib/api'
import { useSociety } from '../lib/useSociety'
import { inr, sqft, PLOT_STATUS } from '../lib/format'
import { Spinner, Stat, ErrorNote, StatusChip } from '../components/ui'

const SECTORS = [
  { n: 1, label: 'Sector 01', range: '1 – 302', colour: '#E8913A' },
  { n: 2, label: 'Sector 02', range: '303 – 364', colour: '#4FA3DC' },
  { n: 3, label: 'Sector 03', range: '365 – 517', colour: '#57A55B' },
  { n: 4, label: 'Sector 04', range: '518 – 823', colour: '#8B7EC8' },
]

export default function MapPage() {
  const { society } = useSociety()
  const navigate = useNavigate()

  const [plots, setPlots] = useState<Plot[] | null>(null)
  const [summary, setSummary] = useState<Summary | null>(null)
  const [error, setError] = useState('')
  const [statusFilter, setStatusFilter] = useState<string>('all')
  const [sectorFilter, setSectorFilter] = useState<number | 'all'>('all')
  const [hover, setHover] = useState<Plot | null>(null)
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
      })
      .catch(() => setError('Could not load the layout.'))
  }, [society])

  const visible = useMemo(() => {
    if (!plots) return []
    return plots.filter((p) => {
      if (statusFilter !== 'all' && p.status !== statusFilter) return false
      if (sectorFilter !== 'all' && p.mapShape?.sector !== sectorFilter) return false
      if (search && !p.plotNo.includes(search.trim())) return false
      return true
    })
  }, [plots, statusFilter, sectorFilter, search])

  // The layout canvas is sized from the seeded polygon coordinates rather than
  // hardcoded, so a real sanctioned layout can replace this without code changes.
  const bounds = useMemo(() => {
    let maxX = 0
    let maxY = 0
    for (const p of plots ?? []) {
      for (const [x, y] of p.mapShape?.points ?? []) {
        if (x > maxX) maxX = x
        if (y > maxY) maxY = y
      }
    }
    return { w: maxX + 80, h: maxY + 80 }
  }, [plots])

  if (error) return <ErrorNote message={error} />
  if (!plots || !summary) return <Spinner label="Loading the layout" />

  const mine = plots.find((p) => p.isMine)

  return (
    <div className="space-y-6">
      <div>
        <p className="text-[11px] font-semibold uppercase tracking-[0.2em] text-gold-600">
          {society?.city} · NMRDA sanctioned
        </p>
        <h1 className="mt-1 font-display text-4xl font-semibold text-olive-950">Layout map</h1>
        <p className="mt-1 max-w-2xl text-sm text-olive-600">
          Every plot in the sanctioned layout, colour-coded live. Tap a plot for its size, facing
          and current status.
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

      {/* Filters */}
      <div className="card flex flex-wrap items-center gap-2 p-3">
        <input
          className="field max-w-[180px]"
          placeholder="Find plot no."
          inputMode="numeric"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <span className="mx-1 h-6 w-px bg-olive-200" />
        <button
          onClick={() => setSectorFilter('all')}
          className={`chip border ${sectorFilter === 'all' ? 'border-olive-700 bg-olive-700 text-white' : 'border-olive-200 bg-white text-olive-700'}`}
        >
          All sectors
        </button>
        {SECTORS.map((s) => (
          <button
            key={s.n}
            onClick={() => setSectorFilter(sectorFilter === s.n ? 'all' : s.n)}
            className={`chip border transition ${
              sectorFilter === s.n ? 'text-white' : 'border-olive-200 bg-white text-olive-700'
            }`}
            style={sectorFilter === s.n ? { background: s.colour, borderColor: s.colour } : undefined}
          >
            <span className="h-2 w-2 rounded-full" style={{ background: s.colour }} />
            {s.label}
            <span className="opacity-60">{s.range}</span>
          </button>
        ))}
        <span className="mx-1 h-6 w-px bg-olive-200" />
        {['all', ...Object.keys(PLOT_STATUS)].map((key) => (
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
                <span
                  className="h-2 w-2 rounded-full"
                  style={{ background: PLOT_STATUS[key]!.dot }}
                />
                {PLOT_STATUS[key]!.label}
              </>
            )}
          </button>
        ))}
        <span className="ml-auto text-xs font-semibold text-olive-500">
          {visible.length} of {plots.length} shown
        </span>
      </div>

      {/* The map itself */}
      <div className="card relative overflow-hidden p-0">
        <div className="max-h-[70vh] overflow-auto p-4">
          <svg
            viewBox={`0 0 ${bounds.w} ${bounds.h}`}
            className="h-auto w-full min-w-[900px]"
            role="img"
            aria-label="Sandesh Nagari 7 plot layout"
          >
            {SECTORS.map((s) => {
              const first = plots.find((p) => p.mapShape?.sector === s.n)
              if (!first?.mapShape) return null
              const y = Math.min(...first.mapShape.points.map(([, py]) => py))
              return (
                <text
                  key={s.n}
                  x={60}
                  y={y - 14}
                  className="font-display"
                  fontSize="22"
                  fontWeight="600"
                  fill={s.colour}
                >
                  {s.label} · plots {s.range}
                </text>
              )
            })}

            {plots.map((p) => {
              const shape = p.mapShape
              if (!shape?.points?.length) return null
              const dimmed = !visible.includes(p)
              const meta = PLOT_STATUS[p.status]
              const points = shape.points.map(([x, y]) => `${x},${y}`).join(' ')
              const [x0, y0] = shape.points[0]!
              const [x2, y2] = shape.points[2]!

              return (
                <g
                  key={p.id}
                  onMouseEnter={() => setHover(p)}
                  onMouseLeave={() => setHover(null)}
                  onClick={() => navigate(`/plots/${p.id}`)}
                  className="cursor-pointer"
                  opacity={dimmed ? 0.12 : 1}
                >
                  <polygon
                    points={points}
                    fill={p.isMine ? '#DFAD35' : (meta?.dot ?? '#999')}
                    fillOpacity={p.isMine ? 0.95 : 0.22}
                    stroke={p.isMine ? '#855F1C' : (meta?.dot ?? '#999')}
                    strokeWidth={p.isMine ? 3 : 1.2}
                  />
                  <text
                    x={(x0 + x2) / 2}
                    y={(y0 + y2) / 2 + 5}
                    textAnchor="middle"
                    fontSize="17"
                    fontWeight="600"
                    fill={p.isMine ? '#2C3520' : '#374327'}
                  >
                    {p.plotNo}
                  </text>
                </g>
              )
            })}
          </svg>
        </div>

        {hover && (
          <div className="pointer-events-none absolute bottom-4 left-4 rounded-xl bg-olive-900/95 px-4 py-3 text-white shadow-lift backdrop-blur">
            <p className="font-display text-lg font-semibold">
              Plot {hover.plotNo}
              {hover.isMine && <span className="ml-2 text-sm text-gold-300">· yours</span>}
            </p>
            <p className="text-xs text-olive-200">
              {hover.phase} · {sqft(hover.areaSqft)} · {hover.facing} facing
              {hover.isCorner ? ' · corner' : ''}
            </p>
            <p className="mt-1 flex items-center gap-2 text-xs">
              <StatusChip status={hover.status} />
              {hover.status === 'available' && hover.price && (
                <span className="font-semibold text-gold-300">{inr(hover.price, { compact: true })}</span>
              )}
            </p>
          </div>
        )}
      </div>

      <p className="text-xs text-olive-500">
        Plot numbers, sector ranges and areas follow the NMRDA-sanctioned layout. Owner names and
        contact details are never shown on the map.
      </p>
    </div>
  )
}
