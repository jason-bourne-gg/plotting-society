import { useMemo, useRef, useState } from 'react'
import type { Plot } from '../lib/api'
import { inr, sqft, PLOT_STATUS } from '../lib/format'

export const SECTORS = [
  { n: 1, label: 'Sector 01', range: '1 – 302', colour: '#E8913A' },
  { n: 2, label: 'Sector 02', range: '303 – 364', colour: '#4FA3DC' },
  { n: 3, label: 'Sector 03', range: '365 – 517', colour: '#57A55B' },
  { n: 4, label: 'Sector 04', range: '518 – 823', colour: '#8B7EC8' },
]

type Props = {
  plots: Plot[]
  /** null shows the whole layout; a number focuses one sector. */
  sector: number | null
  statusFilter: string
  search: string
  onPick?: (plot: Plot) => void
  /** Guests can only act on what is for sale. */
  pickableOnly?: string
}

/**
 * The layout map.
 *
 * 823 plots drawn at once is a wall of numbers nobody can read, so the map
 * draws one sector at a time by default and fits the viewBox to that sector's
 * own bounding box. Cells then come out four times larger with no zoom control
 * to discover, and the sector captions cannot collide with a neighbouring
 * sector's rows — which they did when every sector shared one canvas.
 */
export default function PlotMap({
  plots, sector, statusFilter, search, onPick, pickableOnly,
}: Props) {
  const [hover, setHover] = useState<Plot | null>(null)
  const [zoom, setZoom] = useState(1)
  const scroller = useRef<HTMLDivElement>(null)

  const shown = useMemo(
    () => plots.filter(p => sector === null || p.mapShape?.sector === sector),
    [plots, sector],
  )

  // Fit the canvas to what is actually on it, so a single sector fills the
  // frame instead of sitting in a corner of the whole-project bounding box.
  const box = useMemo(() => {
    let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity
    for (const p of shown) {
      for (const [x, y] of p.mapShape?.points ?? []) {
        if (x < minX) minX = x
        if (y < minY) minY = y
        if (x > maxX) maxX = x
        if (y > maxY) maxY = y
      }
    }
    if (minX === Infinity) return { x: 0, y: 0, w: 100, h: 100 }
    const pad = 30
    return { x: minX - pad, y: minY - pad, w: maxX - minX + pad * 2, h: maxY - minY + pad * 2 }
  }, [shown])

  const matches = (p: Plot) => {
    if (statusFilter !== 'all' && p.status !== statusFilter) return false
    if (search && !p.plotNo.includes(search.trim())) return false
    return true
  }

  const pickable = (p: Plot) => (pickableOnly ? p.status === pickableOnly : true)

  return (
    <div className="card relative overflow-hidden p-0">
      {/* Zoom sits on the canvas, not in a toolbar above it, so the control is
          where the thing it controls is. */}
      <div className="absolute right-3 top-3 z-10 flex items-center gap-1 rounded-xl border border-olive-200 bg-white/90 p-1 shadow-card backdrop-blur">
        <button
          className="grid h-8 w-8 place-items-center rounded-lg text-lg font-semibold text-olive-700 hover:bg-olive-100 disabled:opacity-40"
          onClick={() => setZoom(z => Math.max(1, +(z - 0.5).toFixed(1)))}
          disabled={zoom <= 1}
          aria-label="Zoom out"
        >
          −
        </button>
        <span className="w-10 text-center text-xs font-semibold tabular-nums text-olive-600">
          {zoom.toFixed(1)}×
        </span>
        <button
          className="grid h-8 w-8 place-items-center rounded-lg text-lg font-semibold text-olive-700 hover:bg-olive-100 disabled:opacity-40"
          onClick={() => setZoom(z => Math.min(4, +(z + 0.5).toFixed(1)))}
          disabled={zoom >= 4}
          aria-label="Zoom in"
        >
          +
        </button>
      </div>

      <div ref={scroller} className="max-h-[68vh] overflow-auto p-4">
        <svg
          viewBox={`${box.x} ${box.y} ${box.w} ${box.h}`}
          style={{ width: `${zoom * 100}%`, minWidth: sector === null ? 900 : 560 }}
          className="h-auto"
          role="img"
          aria-label={sector === null ? 'Whole layout' : `Sector 0${sector}`}
        >
          {/* Sector captions only when several sectors share the canvas —
              otherwise the heading above the map already says which one. */}
          {sector === null &&
            SECTORS.map(s => {
              const first = shown.find(p => p.mapShape?.sector === s.n)
              if (!first?.mapShape) return null
              const y = Math.min(...first.mapShape.points.map(([, py]) => py))
              const x = Math.min(...first.mapShape.points.map(([px]) => px))
              return (
                <text key={s.n} x={x} y={y - 18} fontSize="26" fontWeight="700" fill={s.colour}>
                  {s.label} · {s.range}
                </text>
              )
            })}

          {shown.map(p => {
            const shape = p.mapShape
            if (!shape?.points?.length) return null
            const dim = !matches(p)
            const meta = PLOT_STATUS[p.status]
            const [x0, y0] = shape.points[0]!
            const [x2, y2] = shape.points[2]!
            const canPick = pickable(p)

            return (
              <g
                key={p.id}
                opacity={dim ? 0.12 : 1}
                className={canPick ? 'cursor-pointer' : ''}
                onMouseEnter={() => setHover(p)}
                onMouseLeave={() => setHover(null)}
                onClick={() => canPick && onPick?.(p)}
              >
                <rect
                  x={Math.min(x0, x2)}
                  y={Math.min(y0, y2)}
                  width={Math.abs(x2 - x0)}
                  height={Math.abs(y2 - y0)}
                  rx={6}
                  fill={p.isMine ? '#DFAD35' : (meta?.dot ?? '#999')}
                  fillOpacity={p.isMine ? 1 : 0.24}
                  stroke={p.isMine ? '#855F1C' : (meta?.dot ?? '#999')}
                  strokeWidth={p.isMine ? 4 : 1.5}
                />
                <text
                  x={(x0 + x2) / 2}
                  y={(y0 + y2) / 2 + 6}
                  textAnchor="middle"
                  fontSize="19"
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
        <div className="pointer-events-none absolute bottom-4 left-4 max-w-xs rounded-2xl bg-olive-900/95 px-4 py-3 text-white shadow-lift backdrop-blur">
          <p className="font-display text-lg font-semibold">
            Plot {hover.plotNo}
            {hover.isMine && <span className="ml-2 text-sm text-gold-300">· yours</span>}
          </p>
          <p className="mt-0.5 text-xs text-olive-200">
            {hover.phase} · {sqft(hover.areaSqft)} · {hover.facing} facing
            {hover.isCorner ? ' · corner' : ''}
          </p>
          <p className="mt-2 flex items-center gap-2">
            <span
              className="chip text-olive-950"
              style={{ background: PLOT_STATUS[hover.status]?.dot ?? '#999' }}
            >
              {PLOT_STATUS[hover.status]?.label ?? hover.status}
            </span>
            {hover.status === 'available' && hover.price && (
              <span className="font-display text-base font-semibold text-gold-300">
                {inr(hover.price, { compact: true })}
              </span>
            )}
          </p>
        </div>
      )}
    </div>
  )
}

/** The status key, shared by the owner map and the guest page. */
export function MapLegend({ compact = false }: { compact?: boolean }) {
  const keys = compact
    ? ['available', 'booked', 'sold']
    : Object.keys(PLOT_STATUS)
  return (
    <div className="flex flex-wrap items-center gap-x-4 gap-y-1.5 text-xs text-olive-600">
      {keys.map(k => (
        <span key={k} className="inline-flex items-center gap-1.5">
          <span
            className="h-3 w-3 rounded-[4px] border"
            style={{ background: `${PLOT_STATUS[k]!.dot}3d`, borderColor: PLOT_STATUS[k]!.dot }}
          />
          {PLOT_STATUS[k]!.label}
        </span>
      ))}
    </div>
  )
}
