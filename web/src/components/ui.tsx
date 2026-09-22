import type { ReactNode } from 'react'
import { PLOT_STATUS, QUERY_STATUS } from '../lib/format'

export function Spinner({ label = 'Loading' }: { label?: string }) {
  return (
    <div className="flex items-center gap-3 py-12 text-sm text-olive-600" role="status">
      <span className="h-4 w-4 animate-spin rounded-full border-2 border-olive-300 border-t-olive-700" />
      {label}…
    </div>
  )
}

export function Empty({ title, hint, action }: { title: string; hint?: string; action?: ReactNode }) {
  return (
    <div className="card flex flex-col items-center gap-3 px-6 py-14 text-center">
      <div className="grid h-12 w-12 place-items-center rounded-2xl bg-olive-100 text-xl">🗂️</div>
      <p className="font-display text-lg font-semibold text-olive-900">{title}</p>
      {hint && <p className="max-w-sm text-sm text-olive-600">{hint}</p>}
      {action}
    </div>
  )
}

export function ErrorNote({ message }: { message: string }) {
  return (
    <div className="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800">
      {message}
    </div>
  )
}

export function StatusChip({ status }: { status: string }) {
  const meta = PLOT_STATUS[status]
  if (!meta) return <span className="chip bg-neutral-200 text-neutral-700">{status}</span>
  return (
    <span className={`chip ${meta.chip}`}>
      <span className="h-1.5 w-1.5 rounded-full" style={{ background: meta.dot }} />
      {meta.label}
    </span>
  )
}

export function QueryChip({ status }: { status: string }) {
  const meta = QUERY_STATUS[status]
  return (
    <span className={`chip ${meta?.chip ?? 'bg-neutral-200 text-neutral-700'}`}>
      {meta?.label ?? status}
    </span>
  )
}

/** A breached SLA is the one thing in the inbox that should be impossible to miss. */
export function SlaBadge({ breached, due }: { breached: boolean; due?: string }) {
  if (!due) return null
  if (breached) {
    return (
      <span className="chip bg-red-600 text-white shadow-sm">
        <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-white" />
        Past SLA
      </span>
    )
  }
  return <span className="chip bg-olive-100 text-olive-700">Due {new Date(due).toLocaleDateString('en-IN', { day: 'numeric', month: 'short' })}</span>
}

export function Stat({
  label,
  value,
  hint,
  accent = 'olive',
}: {
  label: string
  value: string
  hint?: string
  accent?: 'olive' | 'gold' | 'emerald' | 'red'
}) {
  const accents = {
    olive: 'from-olive-600 to-olive-800',
    gold: 'from-gold-400 to-gold-600',
    emerald: 'from-emerald-500 to-emerald-700',
    red: 'from-red-500 to-red-700',
  }
  return (
    <div className="card relative overflow-hidden p-5">
      <div className={`absolute inset-x-0 top-0 h-1 bg-gradient-to-r ${accents[accent]}`} />
      <p className="text-xs font-semibold uppercase tracking-wider text-olive-500">{label}</p>
      <p className="mt-2 font-display text-3xl font-semibold text-olive-950">{value}</p>
      {hint && <p className="mt-1 text-xs text-olive-500">{hint}</p>}
    </div>
  )
}

export function SectionTitle({ title, sub, action }: { title: string; sub?: string; action?: ReactNode }) {
  return (
    <div className="mb-5 flex flex-wrap items-end justify-between gap-3">
      <div>
        <h2 className="font-display text-2xl font-semibold text-olive-950">{title}</h2>
        {sub && <p className="mt-0.5 text-sm text-olive-600">{sub}</p>}
      </div>
      {action}
    </div>
  )
}

export function Field({
  label,
  error,
  children,
  hint,
}: {
  label: string
  error?: string
  hint?: string
  children: ReactNode
}) {
  return (
    <div>
      <label className="label">{label}</label>
      {children}
      {hint && !error && <p className="mt-1 text-xs text-olive-500">{hint}</p>}
      {error && <p className="mt-1 text-xs font-medium text-red-700">{error}</p>}
    </div>
  )
}
