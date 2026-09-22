import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, ApiFailure, type Plot } from '../../lib/api'
import { useSociety } from '../../lib/useSociety'
import { inr, sqft, PLOT_STATUS } from '../../lib/format'
import { Spinner, SectionTitle, StatusChip, ErrorNote } from '../../components/ui'

export default function Plots() {
  const { society } = useSociety()
  const navigate = useNavigate()

  const [plots, setPlots] = useState<Plot[] | null>(null)
  const [search, setSearch] = useState('')
  const [status, setStatus] = useState('')
  const [saving, setSaving] = useState('')
  const [error, setError] = useState('')

  useEffect(() => {
    if (!society) return
    api.get<{ plots: Plot[] }>(`/api/societies/${society.id}/plots`).then((r) => setPlots(r.plots))
  }, [society])

  const rows = useMemo(() => {
    if (!plots) return []
    return plots
      .filter((p) => (status ? p.status === status : true))
      .filter((p) => (search ? p.plotNo.includes(search.trim()) : true))
      .slice(0, 300)
  }, [plots, search, status])

  /**
   * Status is the field the site office changes constantly — a plot goes on
   * hold, then booked, then sold. Editing it inline is the difference between
   * this being used daily and being abandoned for a spreadsheet.
   */
  async function setPlotStatus(plot: Plot, next: string) {
    setSaving(plot.id)
    setError('')
    try {
      await api.patch(`/api/plots/${plot.id}`, {
        plotNo: plot.plotNo,
        phase: plot.phase ?? '',
        areaSqft: plot.areaSqft ?? null,
        facing: plot.facing ?? '',
        isCorner: plot.isCorner,
        status: next,
        price: plot.price ?? null,
        mapShape: plot.mapShape ?? null,
        notes: '',
      })
      setPlots((prev) => prev?.map((p) => (p.id === plot.id ? { ...p, status: next } : p)) ?? null)
    } catch (err) {
      setError(err instanceof ApiFailure ? err.detail.message : 'Could not update that plot.')
    } finally {
      setSaving('')
    }
  }

  if (!plots) return <Spinner label="Loading plots" />

  return (
    <div className="space-y-5">
      <SectionTitle
        title="Plots"
        sub={`${plots.length} plots across four sectors — change a status inline`}
      />

      {error && <ErrorNote message={error} />}

      <div className="card flex flex-wrap items-center gap-2 p-3">
        <input
          className="field max-w-[200px]"
          placeholder="Find plot no."
          inputMode="numeric"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <button
          onClick={() => setStatus('')}
          className={`chip border ${status === '' ? 'border-olive-700 bg-olive-700 text-white' : 'border-olive-200 bg-white text-olive-700'}`}
        >
          All
        </button>
        {Object.entries(PLOT_STATUS).map(([key, meta]) => (
          <button
            key={key}
            onClick={() => setStatus(key)}
            className={`chip border ${status === key ? 'border-olive-700 bg-olive-700 text-white' : 'border-olive-200 bg-white text-olive-700'}`}
          >
            <span className="h-2 w-2 rounded-full" style={{ background: meta.dot }} />
            {meta.label}
          </button>
        ))}
        <span className="ml-auto text-xs font-semibold text-olive-500">
          showing {rows.length}
        </span>
      </div>

      <div className="card overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead className="bg-olive-50 text-left text-xs uppercase tracking-wider text-olive-600">
              <tr>
                <th className="px-4 py-3">Plot</th>
                <th className="px-4 py-3">Sector</th>
                <th className="px-4 py-3">Area</th>
                <th className="px-4 py-3">Facing</th>
                <th className="px-4 py-3">Price</th>
                <th className="px-4 py-3">Status</th>
                <th className="px-4 py-3">Change to</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-olive-100">
              {rows.map((p) => (
                <tr key={p.id} className={saving === p.id ? 'opacity-50' : 'hover:bg-olive-50'}>
                  <td className="px-4 py-3">
                    <button
                      className="font-semibold text-olive-900 hover:underline"
                      onClick={() => navigate(`/plots/${p.id}`)}
                    >
                      {p.plotNo}
                    </button>
                    {p.isCorner && <span className="ml-2 chip bg-gold-100 text-gold-800">corner</span>}
                  </td>
                  <td className="px-4 py-3 text-olive-600">{p.phase}</td>
                  <td className="px-4 py-3 text-olive-600">{sqft(p.areaSqft)}</td>
                  <td className="px-4 py-3 text-olive-600">{p.facing}</td>
                  <td className="px-4 py-3 font-medium text-olive-800">
                    {p.price ? inr(p.price, { compact: true }) : '—'}
                  </td>
                  <td className="px-4 py-3">
                    <StatusChip status={p.status} />
                  </td>
                  <td className="px-4 py-3">
                    <select
                      className="field py-1.5 text-xs"
                      value={p.status}
                      disabled={saving === p.id}
                      onChange={(e) => void setPlotStatus(p, e.target.value)}
                    >
                      {Object.entries(PLOT_STATUS).map(([key, meta]) => (
                        <option key={key} value={key}>
                          {meta.label}
                        </option>
                      ))}
                    </select>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {plots.length > rows.length && (
        <p className="text-xs text-olive-500">
          Showing the first {rows.length}. Narrow the search to reach the rest.
        </p>
      )}
    </div>
  )
}
