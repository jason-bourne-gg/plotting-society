import { useState } from 'react'
import { NavLink, useLocation, useNavigate } from 'react-router-dom'
import type { ReactNode } from 'react'
import { useAuth } from '../lib/auth'

type NavItem = { to: string; label: string; icon: string; end?: boolean }

const ownerNav: NavItem[] = [
  { to: '/', label: 'Layout map', icon: '🗺️', end: true },
  { to: '/my-plot', label: 'My plot', icon: '📍' },
  { to: '/updates', label: 'Site progress', icon: '📸' },
  { to: '/fund', label: 'Society fund', icon: '₹' },
  { to: '/queries', label: 'My queries', icon: '💬' },
]

const staffNav: NavItem[] = [
  { to: '/admin', label: 'Dashboard', icon: '📊', end: true },
  { to: '/admin/inbox', label: 'Query inbox', icon: '📥' },
  { to: '/admin/leads', label: 'Leads', icon: '🎯' },
  { to: '/admin/plots', label: 'Plots', icon: '🧾' },
  { to: '/admin/updates', label: 'Post update', icon: '📣' },
  { to: '/admin/fund', label: 'Fund ledger', icon: '📒' },
  { to: '/', label: 'Owner view', icon: '👁️', end: true },
]

/** The gradient mark from the Shiv Rudra logo, drawn rather than fetched. */
function Mark() {
  return (
    <svg viewBox="0 0 40 40" className="h-9 w-9 shrink-0" aria-hidden>
      <defs>
        <linearGradient id="mark" x1="0" y1="1" x2="1" y2="0">
          <stop offset="0%" stopColor="#57A55B" />
          <stop offset="35%" stopColor="#4FA3DC" />
          <stop offset="68%" stopColor="#E8913A" />
          <stop offset="100%" stopColor="#DFAD35" />
        </linearGradient>
      </defs>
      {[0, 1, 2].map((i) => (
        <path
          key={i}
          d={`M ${6 + i * 5} 33 A ${16 - i * 5} ${16 - i * 5} 0 0 1 ${34 - i * 5} 33`}
          fill="none"
          stroke="url(#mark)"
          strokeWidth="3.4"
          strokeLinecap="round"
          opacity={1 - i * 0.22}
        />
      ))}
    </svg>
  )
}

export default function Shell({ children }: { children: ReactNode }) {
  const { user, signOut, isStaff } = useAuth()
  const navigate = useNavigate()
  const { pathname } = useLocation()
  const [menuOpen, setMenuOpen] = useState(false)

  const inAdmin = pathname.startsWith('/admin')
  const items = isStaff && inAdmin ? staffNav : ownerNav

  return (
    <div className="min-h-screen">
      <header className="sticky top-0 z-30 border-b border-olive-200/70 bg-cream/85 backdrop-blur-md">
        <div className="mx-auto flex max-w-7xl items-center gap-4 px-4 py-3 sm:px-6">
          <button
            className="flex items-center gap-3 text-left"
            onClick={() => navigate(isStaff ? '/admin' : '/')}
          >
            <Mark />
            <span className="leading-tight">
              <span className="block font-display text-base font-semibold text-olive-950">
                Sandesh Nagari 7
              </span>
              <span className="block text-[11px] font-medium uppercase tracking-[0.14em] text-gold-600">
                Shiv Rudra Group
              </span>
            </span>
          </button>

          <nav className="ml-auto hidden items-center gap-1 lg:flex">
            {items.map((item) => (
              <NavLink
                key={item.to + item.label}
                to={item.to}
                end={item.end}
                className={({ isActive }) =>
                  `rounded-xl px-3 py-2 text-sm font-semibold transition ${
                    isActive
                      ? 'bg-olive-700 text-white shadow-card'
                      : 'text-olive-700 hover:bg-olive-100'
                  }`
                }
              >
                <span className="mr-1.5" aria-hidden>
                  {item.icon}
                </span>
                {item.label}
              </NavLink>
            ))}
          </nav>

          <div className="ml-auto flex items-center gap-2 lg:ml-2">
            {isStaff && !inAdmin && (
              <button className="btn-gold hidden sm:inline-flex" onClick={() => navigate('/admin')}>
                Admin portal
              </button>
            )}
            <div className="hidden text-right sm:block">
              <p className="text-sm font-semibold leading-tight text-olive-900">{user?.name}</p>
              <p className="text-[11px] uppercase tracking-wide text-olive-500">
                {user?.role === 'owner' ? 'Plot owner' : 'Builder'}
              </p>
            </div>
            <button
              className="btn-ghost px-3"
              onClick={() => void signOut()}
              title="Sign out"
              aria-label="Sign out"
            >
              ⏻
            </button>
            <button
              className="btn-ghost px-3 lg:hidden"
              onClick={() => setMenuOpen((v) => !v)}
              aria-label="Menu"
              aria-expanded={menuOpen}
            >
              ☰
            </button>
          </div>
        </div>

        {menuOpen && (
          <nav className="border-t border-olive-200/70 px-4 py-2 lg:hidden">
            {items.map((item) => (
              <NavLink
                key={'m' + item.to + item.label}
                to={item.to}
                end={item.end}
                onClick={() => setMenuOpen(false)}
                className={({ isActive }) =>
                  `block rounded-xl px-3 py-2.5 text-sm font-semibold ${
                    isActive ? 'bg-olive-700 text-white' : 'text-olive-700'
                  }`
                }
              >
                <span className="mr-2" aria-hidden>
                  {item.icon}
                </span>
                {item.label}
              </NavLink>
            ))}
          </nav>
        )}
      </header>

      <main className="mx-auto max-w-7xl animate-fade-up px-4 py-6 sm:px-6 sm:py-8">{children}</main>

      <footer className="mx-auto max-w-7xl px-4 pb-10 pt-4 text-center text-xs text-olive-500 sm:px-6">
        <p>
          Sandesh Nagari 7 · RERA <span className="font-mono">PP1190002601297</span> · NMRDA
          sanctioned · 58 acres · 823 plots
        </p>
        <p className="mt-1">Rui &amp; Banwadi, Wardha Road – MIHAN Corridor, Nagpur</p>
      </footer>
    </div>
  )
}
