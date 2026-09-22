/** Indian-format currency, which is what every number in this app is. */
export function inr(value: number, opts: { compact?: boolean } = {}): string {
  if (opts.compact) {
    // Lakhs and crores, because ₹32,40,000 is harder to read at a glance
    // than ₹32.4 L for anyone used to Indian figures.
    if (Math.abs(value) >= 1_00_00_000) return `₹${(value / 1_00_00_000).toFixed(2)} Cr`
    if (Math.abs(value) >= 1_00_000) return `₹${(value / 1_00_000).toFixed(2)} L`
  }
  return new Intl.NumberFormat('en-IN', {
    style: 'currency',
    currency: 'INR',
    maximumFractionDigits: 0,
  }).format(value)
}

export function sqft(value: number | null | undefined): string {
  if (value == null) return '—'
  return `${new Intl.NumberFormat('en-IN', { maximumFractionDigits: 0 }).format(value)} sq ft`
}

export function shortDate(iso: string | null | undefined): string {
  if (!iso) return '—'
  return new Date(iso).toLocaleDateString('en-IN', {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
  })
}

/** "3 days ago", "in 2 days" — the SLA clock reads better in relative terms. */
export function relative(iso: string | null | undefined): string {
  if (!iso) return '—'
  const diffMs = new Date(iso).getTime() - Date.now()
  const abs = Math.abs(diffMs)
  const rtf = new Intl.RelativeTimeFormat('en', { numeric: 'auto' })

  const units: [Intl.RelativeTimeFormatUnit, number][] = [
    ['day', 86_400_000],
    ['hour', 3_600_000],
    ['minute', 60_000],
  ]
  for (const [unit, ms] of units) {
    if (abs >= ms) return rtf.format(Math.round(diffMs / ms), unit)
  }
  return 'just now'
}

export const PLOT_STATUS: Record<string, { label: string; dot: string; chip: string }> = {
  available: { label: 'Available', dot: '#57A55B', chip: 'bg-emerald-100 text-emerald-800' },
  booked: { label: 'Booked', dot: '#DFAD35', chip: 'bg-amber-100 text-amber-800' },
  sold: { label: 'Sold', dot: '#7C8B99', chip: 'bg-slate-200 text-slate-700' },
  on_hold: { label: 'On hold', dot: '#B08968', chip: 'bg-orange-100 text-orange-800' },
  disputed: { label: 'Disputed', dot: '#DC5A4B', chip: 'bg-red-100 text-red-800' },
  not_for_sale: { label: 'Not for sale', dot: '#4A4A4A', chip: 'bg-neutral-200 text-neutral-700' },
}

export const QUERY_STATUS: Record<string, { label: string; chip: string }> = {
  open: { label: 'Open', chip: 'bg-sky-100 text-sky-800' },
  in_progress: { label: 'In progress', chip: 'bg-violet-100 text-violet-800' },
  waiting_on_owner: { label: 'Waiting on you', chip: 'bg-amber-100 text-amber-800' },
  resolved: { label: 'Resolved', chip: 'bg-emerald-100 text-emerald-800' },
  closed: { label: 'Closed', chip: 'bg-slate-200 text-slate-700' },
}
