import { Link, Outlet, useRouterState } from '@tanstack/react-router'
import clsx from 'clsx'
import { useQuery } from '@tanstack/react-query'
import { CalendarDays, Coffee, Eye, EyeOff, LogOut, Trophy, User as UserIcon, Users } from 'lucide-react'
import { meQuery } from '../api'
import { useLiveUpdates } from '../lib/live'
import { useLogout } from '../lib/logout'
import { useAnonSpoilers } from '../lib/anon'
import { ONYX_URL } from '../lib/links'

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
        24<span className="text-accent"> Cups</span>
      </span>
    </Link>
  )
}

/** Header controls for signed-out visitors. */
function AnonNav() {
  const [spoilers, setSpoilers] = useAnonSpoilers()
  // Return here after signing in, unless we're already on an auth page, in
  // which case carry its redirect along so login/register don't bounce back
  // to each other.
  const redirect = useRouterState({
    select: (st) => {
      const { pathname, href, search } = st.location
      if (pathname === '/login' || pathname === '/register') {
        const r = (search as { redirect?: unknown }).redirect
        return typeof r === 'string' ? r : undefined
      }
      return href
    },
  })
  return (
    <div className="flex items-center gap-2 sm:gap-3">
      <button
        type="button"
        role="switch"
        aria-checked={spoilers}
        onClick={() => setSpoilers(!spoilers)}
        title={spoilers ? "Hide today's door" : "Peek at today's door"}
        className={clsx(
          'inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-xs font-semibold transition',
          spoilers ? 'border-gold bg-gold/15 text-ink' : 'border-line text-ink-2 hover:text-ink',
        )}
      >
        {spoilers ? <Eye size={15} /> : <EyeOff size={15} />}
        <span>Spoilers</span>
        <span className={clsx('rounded-full px-1.5 text-[0.6rem] uppercase', spoilers ? 'bg-gold text-paper' : 'bg-paper-2')}>
          {spoilers ? 'on' : 'off'}
        </span>
      </button>
      <Link to="/login" search={{ redirect }} className="hidden text-sm font-semibold text-ink-2 hover:text-ink sm:inline">
        Sign in
      </Link>
      <Link to="/register" search={{ redirect }} className="btn btn-primary !px-3.5 !py-1.5 text-xs sm:!text-sm">
        Join
      </Link>
    </div>
  )
}

export function Layout() {
  const { data: me } = useQuery(meQuery)
  useLiveUpdates(!!me)
  const logout = useLogout()

  return (
    <div className="flex min-h-dvh flex-col">
      <header className="gold-rule sticky top-0 z-30 bg-paper/85 backdrop-blur-md pt-[env(safe-area-inset-top)]">
        <div className="mx-auto flex h-16 max-w-6xl items-center justify-between gap-4 px-4 sm:px-6">
          <Logo />
          {me === null && <AnonNav />}
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
          24 Cups is an independent, community-run project for comparing notes on the{' '}
          <a href={ONYX_URL} target="_blank" rel="noopener noreferrer" className="underline decoration-gold underline-offset-2 hover:text-ink">
            2026 Onyx Coffee Lab advent calendar
          </a>
          .{' '}
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
