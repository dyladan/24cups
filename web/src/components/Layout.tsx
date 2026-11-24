import { Link, Outlet, useNavigate } from '@tanstack/react-router'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { CalendarDays, Coffee, LogOut, Trophy, User as UserIcon, Users } from 'lucide-react'
import { api, meQuery } from '../api'
import { useLiveUpdates } from '../lib/live'

const nav = [
  { to: '/', label: 'Calendar', icon: CalendarDays },
  { to: '/rankings', label: 'Rankings', icon: Trophy },
  { to: '/brews', label: 'Brews', icon: Coffee },
  { to: '/groups', label: 'Groups', icon: Users },
  { to: '/profile', label: 'Profile', icon: UserIcon },
] as const

export function Logo() {
  return (
    <Link to="/" className="flex items-center gap-2.5">
      <img src="/favicon.svg" alt="" className="size-8" />
      <span className="display text-xl leading-none">
        Advent<span className="text-accent"> Cup</span>
      </span>
    </Link>
  )
}

export function Layout() {
  const { data: me } = useQuery(meQuery)
  useLiveUpdates(!!me)
  const qc = useQueryClient()
  const navigate = useNavigate()

  async function logout() {
    await api('/auth/logout', { method: 'POST' })
    qc.removeQueries({ predicate: (q) => q.queryKey[0] !== 'me' })
    qc.setQueryData(['me'], null)
    navigate({ to: '/login' })
  }

  return (
    <div className="flex min-h-dvh flex-col">
      <header className="gold-rule sticky top-0 z-30 bg-paper/85 backdrop-blur-md pt-[env(safe-area-inset-top)]">
        <div className="mx-auto flex h-16 max-w-6xl items-center justify-between gap-4 px-4 sm:px-6">
          <Logo />
          {me && (
            <nav className="hidden items-center gap-1 md:flex">
              {nav.map((n) => (
                <Link
                  key={n.to}
                  to={n.to}
                  activeOptions={{ exact: n.to === '/' }}
                  className="rounded-full px-3.5 py-2 text-sm font-semibold text-ink-2 transition hover:text-ink data-[status=active]:bg-heading data-[status=active]:text-paper"
                >
                  {n.label}
                </Link>
              ))}
              <button onClick={logout} className="ml-2 rounded-full p-2 text-ink-3 hover:text-ink" title="Sign out" aria-label="Sign out">
                <LogOut size={18} />
              </button>
            </nav>
          )}
        </div>
      </header>

      <main className="mx-auto w-full max-w-6xl flex-1 px-4 pt-6 pb-28 sm:px-6 sm:pt-10 md:pb-16">
        <Outlet />
      </main>

      <footer className="border-t border-gold/40 pb-24 md:pb-0">
        <div className="mx-auto max-w-6xl px-4 py-6 text-xs leading-relaxed text-ink-3 sm:px-6">
          Advent Cup is an independent, community-run project for comparing notes on this year’s coffee advent calendar.
          It is not affiliated with, endorsed by, or sponsored by Onyx Coffee Lab or Fellow. All trademarks belong to
          their respective owners.
        </div>
      </footer>

      {me && (
        <nav className="fixed inset-x-0 bottom-0 z-30 border-t border-gold/40 bg-paper/90 pb-[env(safe-area-inset-bottom)] backdrop-blur-md md:hidden">
          <div className="mx-auto grid max-w-md grid-cols-5">
            {nav.map((n) => (
              <Link
                key={n.to}
                to={n.to}
                activeOptions={{ exact: n.to === '/' }}
                className="group flex flex-col items-center gap-1 py-2.5 text-[0.65rem] font-semibold text-ink-3 data-[status=active]:text-ink"
              >
                <n.icon size={22} className="group-data-[status=active]:text-accent" />
                {n.label}
              </Link>
            ))}
          </div>
        </nav>
      )}
    </div>
  )
}
