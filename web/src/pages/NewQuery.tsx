import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { ApiFailure, api, type Category, type Plot } from '../lib/api'
import { useSociety } from '../lib/useSociety'
import { Field, ErrorNote, Spinner } from '../components/ui'

export default function NewQuery() {
  const { society } = useSociety()
  const navigate = useNavigate()

  const [categories, setCategories] = useState<Category[] | null>(null)
  const [myPlot, setMyPlot] = useState<Plot | null>(null)
  const [category, setCategory] = useState('')
  const [subject, setSubject] = useState('')
  const [body, setBody] = useState('')
  const [fields, setFields] = useState<Record<string, string>>({})
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    api.get<{ categories: Category[] }>('/api/categories').then((r) => setCategories(r.categories))
  }, [])

  useEffect(() => {
    if (!society) return
    api
      .get<{ plots: Plot[] }>(`/api/societies/${society.id}/plots`)
      .then((r) => setMyPlot(r.plots.find((p) => p.isMine) ?? null))
      .catch(() => undefined)
  }, [society])

  const chosen = categories?.find((c) => c.key === category)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    if (!society) return
    setBusy(true)
    setError('')
    setFields({})
    try {
      const res = await api.post<{ id: string }>(`/api/societies/${society.id}/queries`, {
        category,
        subject: subject.trim(),
        body: body.trim(),
        ...(myPlot ? { plotId: myPlot.id } : {}),
      })
      navigate(`/queries/${res.id}`, { replace: true })
    } catch (err) {
      if (err instanceof ApiFailure) {
        setFields(err.detail.fields ?? {})
        if (!err.detail.fields) setError(err.detail.message)
      } else {
        setError('Could not send that. Try again.')
      }
    } finally {
      setBusy(false)
    }
  }

  if (!categories) return <Spinner label="Loading" />

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <button className="text-sm font-semibold text-olive-600 hover:text-olive-800" onClick={() => navigate(-1)}>
        ← Back
      </button>

      <div>
        <h1 className="font-display text-4xl font-semibold text-olive-950">Raise a query</h1>
        <p className="mt-1 text-sm text-olive-600">
          Pick the closest category. Each one carries a response deadline the site office is held to,
          and you will see the clock.
        </p>
      </div>

      <form onSubmit={submit} className="card space-y-5 p-6">
        <div>
          <label className="label">What is it about?</label>
          <div className="grid gap-2 sm:grid-cols-2">
            {categories.map((c) => (
              <button
                key={c.key}
                type="button"
                onClick={() => setCategory(c.key)}
                className={`rounded-xl border p-3 text-left transition ${
                  category === c.key
                    ? 'border-olive-700 bg-olive-700 text-white shadow-card'
                    : 'border-olive-200 bg-white hover:border-olive-400'
                }`}
              >
                <span className="block text-sm font-semibold">{c.label}</span>
                <span
                  className={`block text-xs ${category === c.key ? 'text-olive-100' : 'text-olive-500'}`}
                >
                  Answered within {c.slaDays} days
                </span>
              </button>
            ))}
          </div>
          {fields.category && <p className="mt-1 text-xs font-medium text-red-700">{fields.category}</p>}
        </div>

        {myPlot && (
          <p className="rounded-xl bg-olive-50 px-3.5 py-2.5 text-sm text-olive-700">
            This will be filed against <strong>Plot {myPlot.plotNo}</strong> ({myPlot.phase}).
          </p>
        )}

        <Field label="Subject" error={fields.subject}>
          <input
            className="field"
            required
            maxLength={200}
            value={subject}
            onChange={(e) => setSubject(e.target.value)}
            placeholder="Sale deed scan not received"
          />
        </Field>

        <Field
          label="Details"
          error={fields.body}
          hint="Dates, names and plot numbers get this answered faster."
        >
          <textarea
            className="field min-h-[140px] resize-y"
            required
            value={body}
            onChange={(e) => setBody(e.target.value)}
            placeholder="Registration was completed in March. I still do not have a scanned copy for my records."
          />
        </Field>

        {error && <ErrorNote message={error} />}

        <div className="flex items-center gap-3">
          <button className="btn-primary" disabled={busy || !category}>
            {busy ? 'Sending…' : 'Send to the site office'}
          </button>
          {chosen && (
            <p className="text-xs text-olive-500">
              Expected reply within {chosen.slaDays} days.
            </p>
          )}
        </div>
      </form>
    </div>
  )
}
