import { useEffect, useState } from 'react'
import { api, type SiteUpdate } from '../lib/api'
import { useSociety } from '../lib/useSociety'
import { shortDate, relative, safeUrl } from '../lib/format'
import { Spinner, ErrorNote, Empty } from '../components/ui'

export default function Updates() {
  const { society } = useSociety()
  const [updates, setUpdates] = useState<SiteUpdate[] | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!society) return
    api
      .get<{ updates: SiteUpdate[] }>(`/api/societies/${society.id}/updates`)
      .then((r) => setUpdates(r.updates))
      .catch(() => setError('Could not load site progress.'))
  }, [society])

  if (error) return <ErrorNote message={error} />
  if (!updates) return <Spinner label="Loading site progress" />

  return (
    <div className="space-y-6">
      <div>
        <p className="text-[11px] font-semibold uppercase tracking-[0.2em] text-gold-600">
          From the site office
        </p>
        <h1 className="mt-1 font-display text-4xl font-semibold text-olive-950">Site progress</h1>
        <p className="mt-1 text-sm text-olive-600">
          Dated updates from the ground, newest first.
        </p>
      </div>

      {updates.length === 0 ? (
        <Empty title="No updates posted yet" hint="The site office posts here as work progresses." />
      ) : (
        <ol className="relative space-y-5 border-l-2 border-olive-200 pl-6">
          {updates.map((u, i) => (
            <li key={u.id} className="relative">
              <span
                className={`absolute -left-[31px] top-5 h-3.5 w-3.5 rounded-full border-2 border-cream ${
                  i === 0 ? 'bg-gold-500' : 'bg-olive-400'
                }`}
              />
              <article className="card p-5">
                <div className="flex flex-wrap items-center gap-2">
                  {u.phase && <span className="chip bg-olive-100 text-olive-700">{u.phase}</span>}
                  {!u.publishedAt && <span className="chip bg-amber-100 text-amber-800">Draft</span>}
                  <span className="text-xs text-olive-500">
                    {shortDate(u.publishedAt ?? u.createdAt)} · {relative(u.publishedAt ?? u.createdAt)}
                  </span>
                </div>

                <h2 className="mt-2 font-display text-xl font-semibold text-olive-950">{u.title}</h2>
                {u.body && <p className="mt-2 text-olive-700">{u.body}</p>}

                {u.media.length > 0 && (
                  <div className="mt-4 grid gap-2 sm:grid-cols-3">
                    {u.media.map((m) => (
                      <figure key={m.url} className="overflow-hidden rounded-xl bg-olive-100">
                        <img
                          src={safeUrl(m.url) ?? ''}
                          alt={m.caption ?? u.title}
                          loading="lazy"
                          className="h-36 w-full object-cover transition hover:scale-105"
                        />
                        {m.caption && (
                          <figcaption className="px-2 py-1.5 text-xs text-olive-600">
                            {m.caption}
                          </figcaption>
                        )}
                      </figure>
                    ))}
                  </div>
                )}

                {u.authorName && (
                  <p className="mt-3 text-xs text-olive-500">Posted by {u.authorName}</p>
                )}
              </article>
            </li>
          ))}
        </ol>
      )}
    </div>
  )
}
