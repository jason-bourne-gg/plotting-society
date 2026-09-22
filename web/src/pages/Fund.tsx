import { useEffect, useState } from 'react'
import { api, type FundBalance, type FundEntry } from '../lib/api'
import { useSociety } from '../lib/useSociety'
import { inr, safeUrl, shortDate } from '../lib/format'
import { Spinner, ErrorNote, Stat, SectionTitle, Empty } from '../components/ui'

const HEAD_COLOURS = [
  '#5C6E37', '#C9A227', '#4FA3DC', '#E8913A', '#8B7EC8',
  '#57A55B', '#B08968', '#7C8B99', '#DC5A4B',
]

export default function Fund() {
  const { society } = useSociety()
  const [balance, setBalance] = useState<FundBalance | null>(null)
  const [entries, setEntries] = useState<FundEntry[] | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!society) return
    api
      .get<{ balance: FundBalance; entries: FundEntry[] }>(`/api/societies/${society.id}/fund`)
      .then((r) => {
        setBalance(r.balance)
        setEntries(r.entries)
      })
      .catch(() => setError('Could not load the fund ledger.'))
  }, [society])

  if (error) return <ErrorNote message={error} />
  if (!balance || !entries) return <Spinner label="Loading the ledger" />

  const heads = Object.entries(balance.byHead).sort((a, b) => b[1] - a[1])
  const maxSpend = heads[0]?.[1] ?? 1

  return (
    <div className="space-y-6">
      <div>
        <p className="text-[11px] font-semibold uppercase tracking-[0.2em] text-gold-600">
          Open to every owner
        </p>
        <h1 className="mt-1 font-display text-4xl font-semibold text-olive-950">Society fund</h1>
        <p className="mt-1 max-w-2xl text-sm text-olive-600">
          Every rupee collected and spent. Entries are never edited — a correction appears as its
          own reversal line, so the history always adds up.
        </p>
      </div>

      <div className="grid gap-4 sm:grid-cols-3">
        <Stat label="Collected" value={inr(balance.totalCredit, { compact: true })} accent="emerald" />
        <Stat label="Spent" value={inr(balance.totalDebit, { compact: true })} accent="gold" />
        <Stat
          label="Closing balance"
          value={inr(balance.closing, { compact: true })}
          accent={balance.closing < 0 ? 'red' : 'olive'}
          hint={balance.closing < 0 ? 'Overspent against collections' : 'Available with the society'}
        />
      </div>

      {heads.length > 0 && (
        <section>
          <SectionTitle title="Where it went" sub="Spending by head, largest first" />
          <div className="card space-y-3 p-5">
            {heads.map(([head, amount], i) => (
              <div key={head}>
                <div className="mb-1 flex items-center justify-between text-sm">
                  <span className="font-semibold capitalize text-olive-800">
                    {head.replace(/_/g, ' ')}
                  </span>
                  <span className="font-semibold text-olive-700">{inr(amount, { compact: true })}</span>
                </div>
                <div className="h-2.5 overflow-hidden rounded-full bg-olive-100">
                  <div
                    className="h-full rounded-full transition-all"
                    style={{
                      width: `${Math.max(3, (amount / maxSpend) * 100)}%`,
                      background: HEAD_COLOURS[i % HEAD_COLOURS.length],
                    }}
                  />
                </div>
              </div>
            ))}
          </div>
        </section>
      )}

      <section>
        <SectionTitle title="Ledger" sub={`${entries.length} entries`} />
        {entries.length === 0 ? (
          <Empty title="No entries yet" />
        ) : (
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
                  </p>
                </div>
                {e.credit > 0 ? (
                  <span className="font-semibold text-emerald-700">+{inr(e.credit)}</span>
                ) : (
                  <span className="font-semibold text-olive-700">−{inr(e.debit)}</span>
                )}
                {safeUrl(e.documentUrl) && (
                  <a
                    className="btn-ghost px-3 py-1.5 text-xs"
                    href={safeUrl(e.documentUrl)!}
                    target="_blank"
                    rel="noreferrer noopener"
                  >
                    Bill
                  </a>
                )}
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  )
}
