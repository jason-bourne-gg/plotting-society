import { useCallback, useEffect, useState } from 'react'
import { api, ApiFailure } from '../../lib/api'
import { useSociety } from '../../lib/useSociety'
import { inr, sqft } from '../../lib/format'
import { Spinner, SectionTitle, Stat, ErrorNote, Field } from '../../components/ui'

type Rate = {
  id: string
  sector: string | null
  ratePerSqft: number
  referenceAmount: number
  plotCount: number
  billedCount: number
  updatedAt: string
}

type RatesResponse = {
  rates: Rate[]
  defaultRate: number
  defaultReference: number
  referenceAreaSqft: number
}

const SECTOR_COLOUR: Record<string, string> = {
  'Sector 01': '#E8913A',
  'Sector 02': '#4FA3DC',
  'Sector 03': '#57A55B',
  'Sector 04': '#8B7EC8',
}

export default function Maintenance() {
  const { society } = useSociety()
  const [data, setData] = useState<RatesResponse | null>(null)
  const [drafts, setDrafts] = useState<Record<string, string>>({})
  const [saving, setSaving] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')

  const load = useCallback(async () => {
    if (!society) return
    const res = await api.get<RatesResponse>(`/api/societies/${society.id}/maintenance-rates`)
    setData(res)
    setDrafts({
      __default__: res.defaultRate.toFixed(2),
      ...Object.fromEntries(res.rates.map((r) => [r.sector ?? '', r.ratePerSqft.toFixed(2)])),
    })
  }, [society])

  useEffect(() => {
    void load()
  }, [load])

  async function save(sector: string, key: string) {
    if (!society) return
    const value = Number(drafts[key])
    if (!Number.isFinite(value)) {
      setError('That is not a number.')
      return
    }
    setSaving(key)
    setError('')
    setNotice('')
    try {
      await api.request<{ ratePerSqft: number }>(
        `/api/societies/${society.id}/maintenance-rates`,
        'PUT',
        { sector, ratePerSqft: value },
      )
      await load()
      setNotice(sector ? `${sector} updated.` : 'Default rate updated.')
    } catch (err) {
      setError(err instanceof ApiFailure ? err.detail.message : 'Could not save that rate.')
    } finally {
      setSaving('')
    }
  }

  async function generate() {
    if (!society) return
    setSaving('__generate__')
    setError('')
    setNotice('')
    try {
      const res = await api.post<{ raised: number; total: number }>(
        `/api/societies/${society.id}/maintenance-bills`,
      )
      await load()
      setNotice(
        res.raised === 0
          ? 'Every sold plot already has a bill. Nothing raised.'
          : `Raised ${res.raised} bills totalling ${inr(res.total)}.`,
      )
    } catch (err) {
      setError(err instanceof ApiFailure ? err.detail.message : 'Could not raise the bills.')
    } finally {
      setSaving('')
    }
  }

  if (!data) return <Spinner label="Loading maintenance rates" />

  const totalPlots = data.rates.reduce((n, r) => n + r.plotCount, 0)
  const totalBilled = data.rates.reduce((n, r) => n + r.billedCount, 0)
  const ref = data.referenceAreaSqft

  /** The rate a sector charges, applied to the reference plot — the number the
   *  site office actually quotes to a buyer. */
  const referenceFor = (key: string) => {
    const value = Number(drafts[key])
    return Number.isFinite(value) ? value * ref : 0
  }

  return (
    <div className="space-y-6">
      <SectionTitle
        title="Maintenance rates"
        sub={`One-time charge, priced per square foot. A ${sqft(ref)} plot is the reference everyone quotes.`}
        action={
          <button className="btn-gold" onClick={() => void generate()} disabled={saving !== ''}>
            {saving === '__generate__' ? 'Raising…' : 'Raise bills for unbilled plots'}
          </button>
        }
      />

      <div className="grid gap-4 sm:grid-cols-3">
        <Stat label="Sold plots" value={String(totalPlots)} />
        <Stat label="Billed" value={String(totalBilled)} accent="emerald" />
        <Stat
          label="Not yet billed"
          value={String(Math.max(0, totalPlots - totalBilled))}
          accent={totalPlots - totalBilled > 0 ? 'gold' : 'olive'}
        />
      </div>

      {error && <ErrorNote message={error} />}
      {notice && (
        <p className="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-800">
          {notice}
        </p>
      )}

      {/* Default, used by any sector with no rate of its own */}
      <div className="card p-5">
        <h3 className="font-display text-lg font-semibold text-olive-950">Default rate</h3>
        <p className="mt-1 text-sm text-olive-600">
          Applies to any sector that has no rate of its own.
        </p>
        <div className="mt-4 flex flex-wrap items-end gap-3">
          <Field label="₹ per sq ft">
            <input
              className="field max-w-[140px]"
              type="number"
              step="0.01"
              min="0"
              value={drafts.__default__ ?? ''}
              onChange={(e) => setDrafts((d) => ({ ...d, __default__: e.target.value }))}
            />
          </Field>
          <p className="pb-2.5 text-sm text-olive-600">
            {sqft(ref)} →{' '}
            <strong className="text-olive-900">{inr(referenceFor('__default__'))}</strong>
          </p>
          <button
            className="btn-primary mb-0.5"
            disabled={saving !== ''}
            onClick={() => void save('', '__default__')}
          >
            {saving === '__default__' ? 'Saving…' : 'Save'}
          </button>
        </div>
      </div>

      {/* Per-sector overrides */}
      <div className="grid gap-4 md:grid-cols-2">
        {data.rates.map((r) => {
          const key = r.sector ?? ''
          const unbilled = r.plotCount - r.billedCount
          return (
            <div key={key} className="card overflow-hidden">
              <div
                className="h-1.5"
                style={{ background: SECTOR_COLOUR[key] ?? '#5C6E37' }}
              />
              <div className="p-5">
                <div className="flex items-start justify-between gap-3">
                  <h3 className="font-display text-lg font-semibold text-olive-950">{r.sector}</h3>
                  <span className="chip bg-olive-100 text-olive-700">
                    {r.billedCount}/{r.plotCount} billed
                  </span>
                </div>

                <div className="mt-4 flex flex-wrap items-end gap-3">
                  <Field label="₹ per sq ft">
                    <input
                      className="field max-w-[130px]"
                      type="number"
                      step="0.01"
                      min="0"
                      value={drafts[key] ?? ''}
                      onChange={(e) => setDrafts((d) => ({ ...d, [key]: e.target.value }))}
                    />
                  </Field>
                  <button
                    className="btn-ghost mb-0.5"
                    disabled={saving !== ''}
                    onClick={() => void save(key, key)}
                  >
                    {saving === key ? 'Saving…' : 'Save'}
                  </button>
                </div>

                {/* The check that catches a slipped decimal point before it
                    bills 515 people the wrong amount. */}
                <p className="mt-3 rounded-xl bg-olive-50 px-3.5 py-2.5 text-sm text-olive-700">
                  {sqft(ref)} → <strong>{inr(referenceFor(key))}</strong>
                </p>

                {unbilled > 0 && (
                  <p className="mt-2 text-xs font-medium text-gold-700">
                    {unbilled} sold {unbilled === 1 ? 'plot has' : 'plots have'} no bill yet.
                  </p>
                )}
              </div>
            </div>
          )
        })}
      </div>

      <p className="text-xs text-olive-500">
        Changing a rate only affects bills raised afterwards. A bill already issued keeps the rate
        and area it was calculated from, so an owner is never re-priced after the fact.
      </p>
    </div>
  )
}
