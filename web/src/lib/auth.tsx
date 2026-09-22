import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { api, refreshSession, setSession, storedRefreshToken, type User } from './api'

type AuthState = {
  user: User | null
  loading: boolean
  signIn: (email: string, password: string) => Promise<void>
  signOut: () => Promise<void>
  refreshUser: () => Promise<void>
  isStaff: boolean
}

const AuthContext = createContext<AuthState | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null)
  const [loading, setLoading] = useState(true)

  // On boot, trade the stored refresh token for a session so a reload does not
  // dump the owner back on the login screen.
  useEffect(() => {
    let cancelled = false
    ;(async () => {
      if (!storedRefreshToken()) {
        setLoading(false)
        return
      }
      const ok = await refreshSession()
      if (ok) {
        try {
          const me = await api.get<User>('/api/auth/me')
          if (!cancelled) setUser(me)
        } catch {
          setSession(null, null)
        }
      }
      if (!cancelled) setLoading(false)
    })()
    return () => {
      cancelled = true
    }
  }, [])

  const signIn = useCallback(async (email: string, password: string) => {
    const body = await api.post<{ accessToken: string; refreshToken: string; user: User }>(
      '/api/auth/login',
      { email, password },
    )
    setSession(body.accessToken, body.refreshToken)
    setUser(body.user)
  }, [])

  const signOut = useCallback(async () => {
    try {
      await api.post('/api/auth/logout')
    } catch {
      // Already expired server-side; the local clear below is what matters.
    }
    setSession(null, null)
    setUser(null)
  }, [])

  const refreshUser = useCallback(async () => {
    setUser(await api.get<User>('/api/auth/me'))
  }, [])

  const value = useMemo<AuthState>(
    () => ({
      user,
      loading,
      signIn,
      signOut,
      refreshUser,
      isStaff: user ? user.role !== 'owner' : false,
    }),
    [user, loading, signIn, signOut, refreshUser],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth used outside AuthProvider')
  return ctx
}
