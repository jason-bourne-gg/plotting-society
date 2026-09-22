import { Navigate, Route, Routes, useLocation } from 'react-router-dom'
import { useAuth } from './lib/auth'
import Shell from './components/Shell'
import { Spinner } from './components/ui'

import Explore from './pages/Explore'
import Login from './pages/Login'
import MapPage from './pages/Map'
import PlotDetail from './pages/PlotDetail'
import Updates from './pages/Updates'
import Fund from './pages/Fund'
import Queries from './pages/Queries'
import NewQuery from './pages/NewQuery'
import QueryDetail from './pages/QueryDetail'

import Dashboard from './pages/admin/Dashboard'
import Inbox from './pages/admin/Inbox'
import Leads from './pages/admin/Leads'
import Plots from './pages/admin/Plots'
import PostUpdate from './pages/admin/PostUpdate'
import FundAdmin from './pages/admin/FundAdmin'

/**
 * Three levels of access:
 *
 *   guest    — no account at all. Sees /explore: the layout, what is available,
 *              amenities, recent progress, and an enquiry form.
 *   owner    — signed in. Adds their plot, dues, documents, the society fund
 *              ledger and their own query threads.
 *   builder  — signed in as staff. Everything an owner sees, plus /admin.
 */
function RequireAuth({ children }: { children: JSX.Element }) {
  const { user, loading } = useAuth()
  const location = useLocation()

  if (loading) {
    return (
      <div className="grid min-h-screen place-items-center">
        <Spinner label="Checking your session" />
      </div>
    )
  }
  if (!user) return <Navigate to="/login" replace state={{ from: location.pathname }} />
  return <Shell>{children}</Shell>
}

function RequireStaff({ children }: { children: JSX.Element }) {
  const { user, loading, isStaff } = useAuth()

  if (loading) {
    return (
      <div className="grid min-h-screen place-items-center">
        <Spinner label="Checking your session" />
      </div>
    )
  }
  if (!user) return <Navigate to="/login" replace />
  // An owner who reaches an admin URL is sent to their own view, not shown a
  // permission error for a page that was never theirs.
  if (!isStaff) return <Navigate to="/" replace />
  return <Shell>{children}</Shell>
}

export default function App() {
  const { user, loading } = useAuth()

  return (
    <Routes>
      {/* Guest — no account required */}
      <Route path="/explore" element={<Explore />} />

      <Route
        path="/login"
        element={
          loading ? (
            <div className="grid min-h-screen place-items-center">
              <Spinner />
            </div>
          ) : user ? (
            <Navigate to={user.role === 'owner' ? '/' : '/admin'} replace />
          ) : (
            <Login />
          )
        }
      />

      {/* Owner */}
      <Route
        path="/"
        element={
          loading ? (
            <div className="grid min-h-screen place-items-center">
              <Spinner />
            </div>
          ) : user ? (
            <Shell>
              <MapPage />
            </Shell>
          ) : (
            // An anonymous visitor lands on the public project page, not a
            // login wall. Selling starts before signing in.
            <Navigate to="/explore" replace />
          )
        }
      />
      <Route path="/my-plot" element={<RequireAuth><PlotDetail mine /></RequireAuth>} />
      <Route path="/plots/:plotId" element={<RequireAuth><PlotDetail /></RequireAuth>} />
      <Route path="/updates" element={<RequireAuth><Updates /></RequireAuth>} />
      <Route path="/fund" element={<RequireAuth><Fund /></RequireAuth>} />
      <Route path="/queries" element={<RequireAuth><Queries /></RequireAuth>} />
      <Route path="/queries/new" element={<RequireAuth><NewQuery /></RequireAuth>} />
      <Route path="/queries/:queryId" element={<RequireAuth><QueryDetail /></RequireAuth>} />

      {/* Builder */}
      <Route path="/admin" element={<RequireStaff><Dashboard /></RequireStaff>} />
      <Route path="/admin/inbox" element={<RequireStaff><Inbox /></RequireStaff>} />
      <Route path="/admin/leads" element={<RequireStaff><Leads /></RequireStaff>} />
      <Route path="/admin/plots" element={<RequireStaff><Plots /></RequireStaff>} />
      <Route path="/admin/updates" element={<RequireStaff><PostUpdate /></RequireStaff>} />
      <Route path="/admin/fund" element={<RequireStaff><FundAdmin /></RequireStaff>} />

      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
