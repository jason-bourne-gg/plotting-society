import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, ApiFailure } from '../../lib/api'
import { useSociety } from '../../lib/useSociety'
import { Field, ErrorNote, SectionTitle } from '../../components/ui'

const SECTORS = ['', 'Sector 01', 'Sector 02', 'Sector 03', 'Sector 04']

export default function PostUpdate() {
  const { society } = useSociety()
  const navigate = useNavigate()

  const [title, setTitle] = useState('')
  const [body, setBody] = useState('')
  const [phase, setPhase] = useState('')
  const [photos, setPhotos] = useState<{ url: string; caption?: string }[]>([])
  const [uploading, setUploading] = useState(false)
  const [error, setError] = useState('')
  const [fields, setFields] = useState<Record<string, string>>({})
  const [busy, setBusy] = useState(false)

  /**
   * Photos go straight from the browser to object storage using a presigned
   * URL, so the API never handles image bytes. That is what keeps a
   * scale-to-zero container inside its free CPU allowance.
   */
  async function upload(files: FileList | null) {
    if (!files?.length) return
    setUploading(true)
    setError('')
    try {
      for (const file of Array.from(files).slice(0, 10)) {
        const signed = await api.post<{ uploadUrl: string; publicUrl: string }>('/api/uploads', {
          purpose: 'site_update',
          contentType: file.type,
          sizeBytes: file.size,
        })
        const put = await fetch(signed.uploadUrl, { method: 'PUT', body: file })
        if (!put.ok) throw new Error('upload failed')
        setPhotos((prev) => [...prev, { url: signed.publicUrl }])
      }
    } catch (err) {
      setError(
        err instanceof ApiFailure
          ? err.detail.message
          : 'Could not upload that photo. Check the file size and try again.',
      )
    } finally {
      setUploading(false)
    }
  }

  async function submit(publish: boolean) {
    if (!society) return
    setBusy(true)
    setError('')
    setFields({})
    try {
      await api.post(`/api/societies/${society.id}/updates`, {
        title: title.trim(),
        body: body.trim(),
        phase,
        publish,
        media: photos,
      })
      navigate('/updates')
    } catch (err) {
      if (err instanceof ApiFailure) {
        setFields(err.detail.fields ?? {})
        if (!err.detail.fields) setError(err.detail.message)
      } else {
        setError('Could not post that update.')
      }
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="mx-auto max-w-2xl space-y-5">
      <SectionTitle
        title="Post a site update"
        sub="This is the post that stops the 'any update?' phone calls"
      />

      <form
        onSubmit={(e) => {
          e.preventDefault()
          void submit(true)
        }}
        className="card space-y-5 p-6"
      >
        <Field label="Title" error={fields.title}>
          <input
            className="field"
            required
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="Sector 02 drainage line complete"
          />
        </Field>

        <Field label="Sector (optional)">
          <select className="field" value={phase} onChange={(e) => setPhase(e.target.value)}>
            {SECTORS.map((s) => (
              <option key={s} value={s}>
                {s || 'Whole project'}
              </option>
            ))}
          </select>
        </Field>

        <Field label="What happened" hint="Two or three plain sentences beat a paragraph.">
          <textarea
            className="field min-h-[130px] resize-y"
            value={body}
            onChange={(e) => setBody(e.target.value)}
            placeholder="The 12.0 M road drainage in Sector 02 is laid and tested. Road restoration starts next week."
          />
        </Field>

        <div>
          <label className="label">Photos</label>
          <input
            type="file"
            accept="image/jpeg,image/png,image/webp"
            multiple
            disabled={uploading}
            onChange={(e) => void upload(e.target.files)}
            className="field file:mr-3 file:rounded-lg file:border-0 file:bg-olive-700 file:px-3 file:py-1.5 file:text-white"
          />
          <p className="mt-1 text-xs text-olive-500">
            JPG, PNG or WebP under 10 MB each. Compress before uploading — owners open these on
            mobile data.
          </p>

          {photos.length > 0 && (
            <div className="mt-3 grid grid-cols-3 gap-2">
              {photos.map((p, i) => (
                <div key={p.url} className="relative overflow-hidden rounded-xl">
                  <img src={p.url} alt="" className="h-24 w-full object-cover" />
                  <button
                    type="button"
                    onClick={() => setPhotos((prev) => prev.filter((_, j) => j !== i))}
                    className="absolute right-1 top-1 rounded-lg bg-black/60 px-1.5 text-xs text-white"
                  >
                    ✕
                  </button>
                </div>
              ))}
            </div>
          )}
          {uploading && <p className="mt-2 text-sm text-olive-600">Uploading…</p>}
        </div>

        {error && <ErrorNote message={error} />}

        <div className="flex flex-wrap gap-3">
          <button className="btn-primary" disabled={busy || !title.trim()}>
            {busy ? 'Posting…' : 'Publish to owners'}
          </button>
          <button
            type="button"
            className="btn-ghost"
            disabled={busy || !title.trim()}
            onClick={() => void submit(false)}
          >
            Save as draft
          </button>
        </div>
      </form>
    </div>
  )
}
