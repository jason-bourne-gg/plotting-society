import { useEffect, useState } from 'react'
import { api, type Society } from './api'

/**
 * Resolves the society once and caches it for the session. The app is
 * single-society per deployment today; this is the one place to change when a
 * builder brings a second project.
 */
let cached: Society | null = null

export function useSociety() {
  const [society, setSociety] = useState<Society | null>(cached)
  const [error, setError] = useState('')

  useEffect(() => {
    if (cached) return
    let alive = true
    api
      .get<{ societies: Society[] }>('/api/societies')
      .then((res) => {
        const first = res.societies[0] ?? null
        cached = first
        if (alive) setSociety(first)
      })
      .catch(() => alive && setError('Could not load the society.'))
    return () => {
      alive = false
    }
  }, [])

  return { society, error }
}
