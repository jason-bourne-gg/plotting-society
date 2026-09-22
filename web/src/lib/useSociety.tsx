import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { api, type Society } from './api'

/**
 * The society the app is currently showing.
 *
 * A builder can run several projects — Sandesh Nagari 6 and 7 sit next to each
 * other on the same layout — so this is a selection, not a constant. The first
 * version pinned societies[0] in a module-level variable, which meant a second
 * project would silently never appear and the fix would have touched every
 * page. Everything reads through this context instead.
 *
 * The choice is remembered per browser: staff come back to the project they
 * were working on, not to whichever sorts first.
 */
const STORAGE_KEY = 'ps.society'

type State = {
  society: Society | null
  societies: Society[]
  select: (id: string) => void
  loading: boolean
  error: string
}

const SocietyContext = createContext<State | null>(null)

export function SocietyProvider({ children }: { children: ReactNode }) {
  const [societies, setSocieties] = useState<Society[]>([])
  const [selectedID, setSelectedID] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    let alive = true
    api
      .get<{ societies: Society[] }>('/api/societies')
      .then(res => {
        if (!alive) return
        setSocieties(res.societies)

        let remembered: string | null = null
        try {
          remembered = localStorage.getItem(STORAGE_KEY)
        } catch {
          // Private browsing: fall through to the first society.
        }
        const valid = res.societies.some(s => s.id === remembered)
        setSelectedID(valid ? remembered : (res.societies[0]?.id ?? null))
      })
      .catch(() => alive && setError('Could not load the project.'))
      .finally(() => alive && setLoading(false))
    return () => {
      alive = false
    }
  }, [])

  const select = useCallback((id: string) => {
    setSelectedID(id)
    try {
      localStorage.setItem(STORAGE_KEY, id)
    } catch {
      // Not remembering the choice is survivable; failing to switch is not.
    }
  }, [])

  const value = useMemo<State>(
    () => ({
      societies,
      society: societies.find(s => s.id === selectedID) ?? null,
      select,
      loading,
      error,
    }),
    [societies, selectedID, select, loading, error],
  )

  return <SocietyContext.Provider value={value}>{children}</SocietyContext.Provider>
}

export function useSociety(): State {
  const ctx = useContext(SocietyContext)
  if (!ctx) throw new Error('useSociety used outside SocietyProvider')
  return ctx
}
