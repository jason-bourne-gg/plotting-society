import { useState } from 'react'
import { ApiFailure } from '../lib/api'
import { useAuth } from '../lib/auth'
import { Field } from '../components/ui'

export default function Login() {
  const { signIn } = useAuth()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await signIn(email.trim(), password)
    } catch (err) {
      setError(err instanceof ApiFailure ? err.detail.message : 'Could not sign in. Try again.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="grid min-h-screen lg:grid-cols-2">
      {/* The pitch side: what this replaces, stated plainly. */}
      <section className="relative hidden overflow-hidden bg-olive-800 p-12 text-white lg:flex lg:flex-col lg:justify-between">
        <div
          className="pointer-events-none absolute inset-0 opacity-30"
          style={{
            backgroundImage:
              'radial-gradient(700px 400px at 80% 10%, rgba(223,173,53,.5), transparent 60%),' +
              'radial-gradient(600px 500px at 10% 90%, rgba(79,163,220,.35), transparent 60%)',
          }}
        />
        <div className="relative">
          <p className="text-[11px] font-semibold uppercase tracking-[0.3em] text-gold-300">
            Shiv Rudra Group
          </p>
          <h1 className="mt-4 font-display text-5xl font-semibold leading-[1.05]">
            Sandesh Nagari <span className="text-gold-300">7</span>
          </h1>
          <p className="mt-4 max-w-md text-olive-100">
            The address that Nagpur is heading towards. 823 plots across 58 NMRDA-sanctioned acres
            at Rui &amp; Banwadi, on the Wardha Road–MIHAN corridor.
          </p>
        </div>

        <div className="relative grid gap-4 sm:grid-cols-3">
          {[
            ['823', 'Plots'],
            ['58', 'Acres'],
            ['4', 'Sectors'],
          ].map(([value, label]) => (
            <div key={label} className="rounded-2xl bg-white/10 p-4 backdrop-blur">
              <p className="font-display text-3xl font-semibold text-gold-300">{value}</p>
              <p className="text-xs uppercase tracking-widest text-olive-100">{label}</p>
            </div>
          ))}
        </div>

        <p className="relative max-w-md text-sm text-olive-200">
          Your plot, the site&apos;s progress, the society&apos;s accounts and every question you
          have raised — without a phone call to the sales desk.
        </p>
      </section>

      {/* The form side. */}
      <section className="flex items-center justify-center px-5 py-16">
        <div className="w-full max-w-sm animate-fade-up">
          <div className="lg:hidden">
            <p className="text-[11px] font-semibold uppercase tracking-[0.25em] text-gold-600">
              Shiv Rudra Group
            </p>
            <h1 className="mt-2 font-display text-4xl font-semibold text-olive-950">
              Sandesh Nagari 7
            </h1>
          </div>

          <h2 className="mt-8 font-display text-2xl font-semibold text-olive-950 lg:mt-0">
            Sign in
          </h2>
          <p className="mt-1 text-sm text-olive-600">
            Use the email the site office invited you with.
          </p>

          <form onSubmit={submit} className="mt-6 space-y-4">
            <Field label="Email">
              <input
                className="field"
                type="email"
                autoComplete="username"
                required
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="you@example.com"
              />
            </Field>

            <Field label="Password">
              <input
                className="field"
                type="password"
                autoComplete="current-password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="••••••••••"
              />
            </Field>

            {error && (
              <p className="rounded-xl border border-red-200 bg-red-50 px-3.5 py-2.5 text-sm text-red-800">
                {error}
              </p>
            )}

            <button type="submit" className="btn-primary w-full" disabled={busy}>
              {busy ? 'Signing in…' : 'Sign in'}
            </button>
          </form>

          <div className="mt-8 rounded-2xl border border-dashed border-olive-300 bg-white/60 p-4 text-xs text-olive-600">
            <p className="font-semibold uppercase tracking-wider text-olive-700">Demo logins</p>
            <ul className="mt-2 space-y-1 font-mono text-[11px]">
              <li>rohit.deshmukh@example.com — plot owner</li>
              <li>office@shivrudragroup.in — builder admin</li>
            </ul>
            <p className="mt-2 font-mono text-[11px]">password: sandesh-demo-2026</p>
          </div>

          <p className="mt-6 text-xs text-olive-500">
            No account? The site office creates it when your plot is registered.
          </p>
        </div>
      </section>
    </div>
  )
}
