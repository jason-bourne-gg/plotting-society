type Landmark = { name: string; minutes: number }

type Props = {
  name: string
  address?: string
  city?: string
  latitude?: number
  longitude?: number
  mapLabel?: string
  landmarks?: Landmark[]
}

/**
 * Where the land actually is.
 *
 * When coordinates exist the site is drawn on an OpenStreetMap frame — no API
 * key, no billing account, no script on the page. When they do not, the card
 * falls back to the address and a search link rather than dropping a pin in
 * roughly the right district: a wrong pin looks authoritative in a way a plain
 * address does not, and this is the one number a buyer may drive to.
 */
export default function LocationCard({
  name, address, city, latitude, longitude, mapLabel, landmarks = [],
}: Props) {
  const query = encodeURIComponent([mapLabel || name, address, city].filter(Boolean).join(', '))
  const hasPin = typeof latitude === 'number' && typeof longitude === 'number'

  const directions = hasPin
    ? `https://www.google.com/maps/dir/?api=1&destination=${latitude},${longitude}`
    : `https://www.google.com/maps/dir/?api=1&destination=${query}`
  const open = hasPin
    ? `https://www.google.com/maps/search/?api=1&query=${latitude},${longitude}`
    : `https://www.google.com/maps/search/?api=1&query=${query}`

  // A tight box around the pin: enough context to place it against the
  // highway without zooming out to the whole district.
  const span = 0.012
  const embed = hasPin
    ? `https://www.openstreetmap.org/export/embed.html?bbox=${longitude! - span}%2C${latitude! - span}%2C${longitude! + span}%2C${latitude! + span}&layer=mapnik&marker=${latitude}%2C${longitude}`
    : null

  const sorted = [...landmarks].sort((a, b) => a.minutes - b.minutes)

  return (
    <section id="location" className="scroll-mt-8">
      <h2 className="font-display text-3xl font-semibold text-olive-950">Where it is</h2>
      <p className="mt-1 text-sm text-olive-600">{address}</p>

      <div className="mt-5 grid gap-4 lg:grid-cols-[1.25fr_1fr]">
        <div className="card overflow-hidden">
          {embed ? (
            <iframe
              title={`Map of ${name}`}
              src={embed}
              className="h-[340px] w-full border-0"
              loading="lazy"
              referrerPolicy="no-referrer"
            />
          ) : (
            <div className="flex h-[340px] flex-col items-center justify-center gap-3 bg-gradient-to-br from-olive-100 to-olive-200 p-8 text-center">
              <span className="grid h-14 w-14 place-items-center rounded-2xl bg-white/70 text-2xl">
                📍
              </span>
              <p className="font-display text-xl font-semibold text-olive-900">
                {mapLabel || name}
              </p>
              <p className="max-w-sm text-sm text-olive-700">{address}</p>
              <p className="text-xs text-olive-600">
                Open it in Maps for the exact approach road.
              </p>
            </div>
          )}

          <div className="flex flex-wrap gap-2 border-t border-olive-100 p-4">
            <a className="btn-primary" href={directions} target="_blank" rel="noreferrer noopener">
              Get directions
            </a>
            <a className="btn-ghost" href={open} target="_blank" rel="noreferrer noopener">
              Open in Google Maps
            </a>
          </div>
        </div>

        {sorted.length > 0 && (
          <div className="card p-5">
            <h3 className="font-display text-lg font-semibold text-olive-950">
              What is close by
            </h3>
            <p className="mt-0.5 text-xs uppercase tracking-wide text-olive-500">
              Driving time from the gate
            </p>
            <ul className="mt-4 space-y-2.5">
              {sorted.map(l => (
                <li key={l.name} className="flex items-baseline gap-3">
                  <span className="w-14 shrink-0 text-right font-display text-lg font-semibold text-gold-600 tabular-nums">
                    {l.minutes} min
                  </span>
                  <span className="h-px flex-1 translate-y-[-3px] bg-olive-200" />
                  <span className="text-sm text-olive-800">{l.name}</span>
                </li>
              ))}
            </ul>
          </div>
        )}
      </div>
    </section>
  )
}
