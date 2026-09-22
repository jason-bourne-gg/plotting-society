/**
 * One fetch wrapper for the whole app.
 *
 * It holds the access token in memory and the refresh token in localStorage,
 * and transparently retries once after refreshing — so a 15-minute access
 * token never surfaces to a component as an error.
 */

const REFRESH_KEY = 'ps.refresh'

export type ApiError = {
  code: string
  message: string
  fields?: Record<string, string>
  status: number
}

export class ApiFailure extends Error {
  readonly detail: ApiError
  constructor(detail: ApiError) {
    super(detail.message)
    this.detail = detail
  }
}

let accessToken: string | null = null
let refreshing: Promise<boolean> | null = null

export function setSession(access: string | null, refresh: string | null) {
  accessToken = access
  try {
    if (refresh) localStorage.setItem(REFRESH_KEY, refresh)
    else localStorage.removeItem(REFRESH_KEY)
  } catch {
    // Private browsing: the session simply does not survive a reload.
  }
}

export function storedRefreshToken(): string | null {
  try {
    return localStorage.getItem(REFRESH_KEY)
  } catch {
    return null
  }
}

export function hasAccessToken(): boolean {
  return accessToken !== null
}

/** Exchanges the stored refresh token for a new pair. */
export async function refreshSession(): Promise<boolean> {
  // Collapse concurrent refreshes: a page with four widgets must not fire four.
  if (refreshing) return refreshing

  refreshing = (async () => {
    const token = storedRefreshToken()
    if (!token) return false
    try {
      const res = await fetch('/api/auth/refresh', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ refreshToken: token }),
      })
      if (!res.ok) {
        setSession(null, null)
        return false
      }
      const body = await res.json()
      setSession(body.accessToken, body.refreshToken)
      return true
    } catch {
      return false
    } finally {
      refreshing = null
    }
  })()

  return refreshing
}

async function request<T>(path: string, init: RequestInit = {}, retry = true): Promise<T> {
  const headers = new Headers(init.headers)
  if (init.body && !headers.has('content-type')) {
    headers.set('content-type', 'application/json')
  }
  if (accessToken) headers.set('authorization', `Bearer ${accessToken}`)

  const res = await fetch(path, { ...init, headers })

  if (res.status === 401 && retry && storedRefreshToken()) {
    if (await refreshSession()) return request<T>(path, init, false)
  }

  if (res.status === 204) return undefined as T

  const text = await res.text()
  const body = text ? JSON.parse(text) : null

  if (!res.ok) {
    const err = body?.error ?? {}
    throw new ApiFailure({
      code: err.code ?? 'unknown',
      message: err.message ?? `Request failed (${res.status})`,
      fields: err.fields,
      status: res.status,
    })
  }
  return body as T
}

export const api = {
  get: <T,>(path: string) => request<T>(path),
  post: <T,>(path: string, body?: unknown) =>
    request<T>(path, { method: 'POST', body: body ? JSON.stringify(body) : undefined }),
  patch: <T,>(path: string, body: unknown) =>
    request<T>(path, { method: 'PATCH', body: JSON.stringify(body) }),
}

// ------------------------------------------------------------------- types

export type Role = 'super_admin' | 'builder_admin' | 'builder_staff' | 'owner'

export type User = {
  id: string
  builderId?: string
  email: string
  phone?: string
  name: string
  role: Role
  avatarUrl?: string
  currentAddress?: string
  city?: string
  directoryOptIn: boolean
  isActive: boolean
  createdAt: string
}

export type Society = {
  id: string
  builderId: string
  builderName: string
  name: string
  slug: string
  city?: string
  address?: string
  layoutImageUrl?: string
  layoutWidth?: number
  layoutHeight?: number
  reraNumber?: string
}

export type MapShape = {
  points: [number, number][]
  sector?: number
  colour?: string
}

export type Plot = {
  id: string
  plotNo: string
  phase?: string
  areaSqft?: number
  facing?: string
  isCorner: boolean
  status: string
  price?: number
  mapShape?: MapShape
  isMine: boolean
}

export type Due = {
  id: string
  periodLabel: string
  amountDue: number
  amountPaid: number
  dueDate?: string
  paidOn?: string
  receiptUrl?: string
}

export type PlotDetail = Plot & {
  notes?: string
  ownerName?: string
  dues?: Due[]
  documents?: { id: string; docType: string; title: string; url: string }[]
}

export type Summary = {
  total: number
  sold: number
  available: number
  booked: number
  onHold: number
}

export type Query = {
  id: string
  societyId: string
  plotId?: string
  plotNo?: string
  raisedBy: string
  raisedByName: string
  category: string
  subject: string
  status: string
  priority: string
  slaDueAt?: string
  resolvedAt?: string
  createdAt: string
  updatedAt: string
  breached: boolean
}

export type QueryMessage = {
  id: string
  authorId: string
  authorName: string
  body: string
  attachmentUrl?: string
  isInternal: boolean
  createdAt: string
}

export type Category = { key: string; label: string; slaDays: number }

export type FundEntry = {
  id: string
  entryDate: string
  head: string
  description: string
  credit: number
  debit: number
  documentUrl?: string
  reversesId?: string
  createdBy?: string
  createdAt: string
}

export type FundBalance = {
  totalCredit: number
  totalDebit: number
  closing: number
  byHead: Record<string, number>
}

export type SiteUpdate = {
  id: string
  title: string
  body?: string
  phase?: string
  publishedAt?: string
  authorName?: string
  createdAt: string
  media: { url: string; caption?: string }[]
}
