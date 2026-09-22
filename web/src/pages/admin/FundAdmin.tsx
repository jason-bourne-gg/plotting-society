import { useCallback, useEffect, useState } from 'react'
import { api, ApiFailure, type FundBalance, type FundEntry } from '../../lib/api'
import { useSociety } from '../../lib/useSociety'
import { inr, shortDate } from '../../lib/format'
import { Spinner, Stat, SectionTitle, Field, ErrorNote } from '../../components/ui'

const HEADS = [
  'collections', 'security', 'roads', 'drainage', 'water', 'electrification',
  'stp', 'landscaping', 'amenities', 'admin', 'other',
]

export default function FundAdmin() {
  const { society } = useSociety()
  const [balance, setBalance] = useState<FundBalance | null>(null)
  const [entries, setEntries] = useState<FundEntry[]>([])

  const [entryDate, setEntryDate] = useState(() => new Date().toISOString().slice(0, 10))
  const [head, setHead] = useState('collections')
  const [description, setDescription] = useState('')
  const [direction, setDirection] = useState<'in' | 'out'>('out')
  const [amount, setAmount] = useState('')
  const [fields, setFields] = useState<Record<string, string>>({})
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const load = useCallback(async () => {
    if (!society) return
    const r = await api.get<{ balance: FundBalance; entries: FundEntry[] }>(
      `/api/societies/${society.id}/fund`,
    )
    setBalance(r.balance)
    setEntries(r.entries)
  }, [society])

  useEffect(() => {
    void load()
  }, [load])

  async function add(e: React.FormEvent) {
    e.preventDefault()
    if (!society) return
    setBusy(true)
    setError('')
    setFields({})
    const value = Number(amount)
    try {
      await api.post(`/api/societies/${society.id}/fund`, {
        entryDate,
        head,
        description: description.trim(),
        credit: direction === 'in' ? value : 0,
        debit: direction === 'out' ? value : 0,
      })
      setDescription('')
      setAmount('')
      await load()
    } catch (err) {
      if (err instanceof ApiFailure) {
        setFields(err.detail.fields ?? {})
        if (!err.detail.fields) setError(err.detail.message)
      } else {
        setError('Could not add that entry.')
      }
    } finally {
      setBusy(false)
    }
  }

  /** Corrections are reversals, never edits — that is the whole point of the ledger. */
  async function reverse(entry: FundEntry) {
    const reason = window.prompt(`Reverse "${entry.description}"?\n\nWhy?`)
    if (!reason?.trim()) return
    try {
      await api.post(`/api/fund/${entry.id}/reverse`, { reason: reason.trim() })
      await load()
    } catch (err) {
      setError(err instanceof ApiFailure ? err.detail.message : 'Could not reverse that entry.')
    }
  }

  if (!balance) return <Spinner label="Loading the ledger" />

  return (
    <div className="space-y-6">
      <SectionTitle
        title="Fund ledger"
        sub="Every owner can read this. Entries are never edited — a mistake is corrected with a reversal."
      />

      <div className="grid gap-4 sm:grid-cols-3">
        <Stat label="Collected" value={inr(balance.totalCredit, { compact: true })} accent="emerald" />
        <Stat label="Spent" value={inr(balance.totalDebit, { compact: true })} accent="gold" />
        <Stat
          label="Closing balance"
          value={inr(balance.closing, { compact: true })}
          accent={balance.closing < 0 ? 'red' : 'olive'}
        />
      </div>

      <form onSubmit={add} className="card space-y-4 p-5">
        <h3 className="font-display text-lg font-semibold text-olive-950">Add an entry</h3>

        <div className="grid gap-4 sm:grid-cols-4">
          <Field label="Date" error={fields.entryDate}>
            <input
              className="field"
              type="date"
              required
              value={entryDate}
              onChange={(e) => setEntryDate(e.target.value)}
            />
          </Field>
          <Field label="Head" error={fields.head}>
            <select className="field" value={head} onChange={(e) => setHead(e.target.value)}>
              {HEADS.map((h) => (
                <option key={h} value={h}>
                  {h.replace(/_/g, ' ')}
                </option>
              ))}
            </select>
          </Field>
          <Field label="Direction">
            <select
              className="field"
              value={direction}
              onChange={(e) => setDirection(e.target.value as 'in' | 'out')}
            >
              <option value="in">Money in</option>
              <option value="out">Money out</option>
            </select>
          </Field>
          <Field label="Amount (₹)" error={fields.amount}>
            <input
              className="field"
              type="number"
              min="1"
              step="1"
              required
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
              placeholder="66000"
            />
          </Field>
        </div>

        <Field label="Description" error={fields.description}>
          <input
            className="field"
            required
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="Security agency, 6 guards — Oct to Dec"
          />
        </Field>

        {error && <ErrorNote message={error} />}

        <button className="btn-primary" disabled={busy}>
          {busy ? 'Adding…' : 'Add to ledger'}
        </button>
      </form>

      <div className="card divide-y divide-olive-100 overflow-hidden">
        {entries.map((e) => (
          <div key={e.id} className="flex flex-wrap items-center gap-3 p-4">
            <div className="w-20 shrink-0 text-xs font-semibold text-olive-500">
              {shortDate(e.entryDate)}
            </div>
            <div className="min-w-0 flex-1">
              <p className="font-medium text-olive-950">{e.description}</p>
              <p className="text-xs uppercase tracking-wide text-olive-500">
                {e.head.replace(/_/g, ' ')}
                {e.reversesId && ' · reversal'}
                {e.createdBy && ` · ${e.createdBy}`}
              </p>
            </div>
            {e.credit > 0 ? (
              <span className="font-semibold text-emerald-700">+{inr(e.credit)}</span>
            ) : (
              <span className="font-semibold text-olive-700">−{inr(e.debit)}</span>
            )}
            {!e.reversesId && (
              <button className="btn-ghost px-3 py-1.5 text-xs" onClick={() => void reverse(e)}>
                Reverse
              </button>
            )}
          </div>
        ))}
      </div>
    </div>
  )
}
