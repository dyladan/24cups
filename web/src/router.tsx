import type { QueryClient } from '@tanstack/react-query'
import { createRootRouteWithContext, createRoute, createRouter, redirect, Link } from '@tanstack/react-router'
import { meQuery } from './api'
import { Layout } from './components/Layout'
import { Spinner } from './components/ui'
import { LoginPage, RegisterPage } from './routes/auth'
import { CalendarPage } from './routes/calendar'
import { CoffeePage } from './routes/coffee'
import { RankingsPage } from './routes/rankings'
import { BrewsPage } from './routes/brews'
import { NewBrewPage, EditBrewPage } from './routes/brew-form'
import { GroupsPage, GroupPage, JoinPage } from './routes/groups'
import { ProfilePage } from './routes/profile'

interface Ctx {
  queryClient: QueryClient
}

const rootRoute = createRootRouteWithContext<Ctx>()({
  component: Layout,
  notFoundComponent: () => (
    <div className="py-20 text-center">
      <p className="display text-6xl">404</p>
      <p className="mt-3 text-ink-2">That page isn’t in the box.</p>
      <Link to="/" className="btn btn-primary mt-6">
        Back to the calendar
      </Link>
    </div>
  ),
})

const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/login',
  validateSearch: (s: Record<string, unknown>): { redirect?: string } => ({
    redirect: typeof s.redirect === 'string' ? s.redirect : undefined,
  }),
  component: LoginPage,
})

const registerRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/register',
  validateSearch: (s: Record<string, unknown>): { redirect?: string } => ({
    redirect: typeof s.redirect === 'string' ? s.redirect : undefined,
  }),
  component: RegisterPage,
})

const appRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: 'app',
  beforeLoad: async ({ context, location }) => {
    const me = await context.queryClient.ensureQueryData(meQuery)
    if (!me) throw redirect({ to: '/login', search: { redirect: location.href } })
    return { me }
  },
  pendingComponent: Spinner,
})

// Pages signed-out visitors can see too.
const publicRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: 'public',
  beforeLoad: async ({ context }) => {
    await context.queryClient.ensureQueryData(meQuery)
  },
  pendingComponent: Spinner,
})

const calendarRoute = createRoute({ getParentRoute: () => publicRoute, path: '/', component: CalendarPage })

const coffeeRoute = createRoute({
  getParentRoute: () => publicRoute,
  path: '/coffee/$coffeeId',
  params: {
    parse: (p) => ({ coffeeId: Number(p.coffeeId) }),
    stringify: (p) => ({ coffeeId: String(p.coffeeId) }),
  },
  component: CoffeePage,
})

const rankingsRoute = createRoute({ getParentRoute: () => appRoute, path: '/rankings', component: RankingsPage })
const brewsRoute = createRoute({ getParentRoute: () => appRoute, path: '/brews', component: BrewsPage })

const newBrewRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/brews/new',
  validateSearch: (s: Record<string, unknown>): { coffeeId?: number } => ({
    coffeeId: s.coffeeId != null && Number.isFinite(Number(s.coffeeId)) ? Number(s.coffeeId) : undefined,
  }),
  component: NewBrewPage,
})

const editBrewRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/brews/$brewId/edit',
  params: {
    parse: (p) => ({ brewId: Number(p.brewId) }),
    stringify: (p) => ({ brewId: String(p.brewId) }),
  },
  component: EditBrewPage,
})

const groupsRoute = createRoute({ getParentRoute: () => appRoute, path: '/groups', component: GroupsPage })
const groupRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/groups/$groupId',
  params: {
    parse: (p) => ({ groupId: Number(p.groupId) }),
    stringify: (p) => ({ groupId: String(p.groupId) }),
  },
  component: GroupPage,
})
const joinRoute = createRoute({ getParentRoute: () => appRoute, path: '/join/$code', component: JoinPage })
const profileRoute = createRoute({ getParentRoute: () => appRoute, path: '/profile', component: ProfilePage })

const routeTree = rootRoute.addChildren([
  loginRoute,
  registerRoute,
  publicRoute.addChildren([calendarRoute, coffeeRoute]),
  appRoute.addChildren([
    rankingsRoute,
    brewsRoute,
    newBrewRoute,
    editBrewRoute,
    groupsRoute,
    groupRoute,
    joinRoute,
    profileRoute,
  ]),
])

export function createAppRouter(queryClient: QueryClient) {
  return createRouter({
    routeTree,
    context: { queryClient },
    defaultPreload: 'intent',
    defaultPreloadStaleTime: 0,
    scrollRestoration: true,
  })
}
