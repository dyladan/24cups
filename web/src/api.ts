import { queryOptions, useMutation, useQueryClient } from '@tanstack/react-query'
import type { CoffeeView } from './lib/anon'

// ---------- types ----------

export type SpoilerMode = 'rated' | 'opened' | 'always'

export interface User {
  id: number
  username: string
  displayName: string
  /** When tasting notes and community scores appear. */
  spoilerMode: SpoilerMode
  tempUnit: 'C' | 'F'
  createdAt: string
}

export interface Passkey {
  id: number
  name: string
  createdAt: string
  lastUsedAt: string | null
}

export interface SecurityInfo {
  /** Set when passkeys are on; they only work on this hostname. */
  passkeyRpId?: string
  hasPassword: boolean
  passkeys: Passkey[]
  recoveryCodesLeft: number
}

export interface CoffeeInfo {
  name: string
  origin: string
  region: string
  producer: string
  process: string
  variety: string
  elevation: string
  roastLevel: string
  tastingNotes: string
  description: string
}

export interface Stats {
  avgRating: number | null
  ratingCount: number
  brewCount: number
  opens: number
  favorites: number
}

export interface Coffee {
  id: number
  day: number
  opened: boolean
  openedAt: string | null
  favorited: boolean
  info: CoffeeInfo | null
  stats: Stats | null
  /** False while notes and scores are hidden (opened but not yet rated). */
  revealed: boolean
  myAvg: number | null
  myBrewCount: number
  /** Today's name and origin while its door is sealed (not a spoiler). */
  preview?: { name: string; origin: string }
}

export interface GroupStats extends Stats {
  groupId: number
  groupName: string
}

export interface FlavorCount {
  name: string
  /** Tasters who used it. */
  count: number
  /** The viewer tagged it on one of their own brews. */
  mine: boolean
}

export interface Flavors {
  /** People who tagged at least one flavor. */
  tasters: number
  /** Null until enough people have tagged it, or while tasting blind. */
  top: FlavorCount[] | null
}

export interface CoffeeDetail extends Coffee {
  distribution?: number[]
  groupStats?: GroupStats[]
  /** Absent on sealed doors. */
  flavors?: Flavors
}

export interface RecipeFields {
  method: string
  doseG: number | null
  ratio: number
  tempC: number | null
  targetTimeS: number | null
}

export interface Recipe extends RecipeFields {
  id: number
  name: string
  notes: string
}

export interface Grinder {
  id: number
  model: string
  nickname: string
  burrsets: string[]
  notes: string
}

export interface Brew extends RecipeFields {
  id: number
  userId: number
  username: string
  displayName: string
  mine: boolean
  coffeeId: number
  coffeeDay: number
  coffeeName: string | null
  recipeId: number | null
  actualTimeS: number | null
  grinderId: number | null
  grinderLabel: string
  burrset: string
  grindSetting: string
  rating: number | null
  notes: string
  /** Lowercase flavor tags. */
  descriptors: string[]
  isPublic: boolean
  excludeFromGlobal: boolean
  brewedAt: string
}

export interface BrewInput extends RecipeFields {
  coffeeId: number
  recipeId: number | null
  actualTimeS: number | null
  grinderId: number | null
  burrset: string
  grindSetting: string
  rating: number | null
  notes: string
  descriptors: string[]
  isPublic: boolean
  excludeFromGlobal: boolean
  brewedAt?: string
}

export interface Group {
  id: number
  name: string
  description: string
  isPublic: boolean
  ownerId: number
  ownerName: string
  memberCount: number
  role: '' | 'member' | 'owner'
  inviteCode?: string
  createdAt: string
}

export interface GroupDetail extends Group {
  members: {
    userId: number
    username: string
    /** Name shown in this group: groupName if set, else profileName. */
    displayName: string
    /** The member's own name for this group; '' if unset. */
    groupName: string
    profileName: string
    role: string
    joinedAt: string
    opens: number
    brews: number
  }[]
  coffees: { coffeeId: number; day: number; name: string | null; stats: Stats | null; revealed: boolean }[]
  brews: Brew[]
}

export interface Catalog {
  methods: string[]
  grinders: { model: string; type: string; stockBurrs: string; burrSizeMm?: number; burrGeometry: string }[]
  burrsets: string[]
  flavors: { group: string; items: string[] }[]
}

// ---------- fetch ----------

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

/** Identifies this tab so the server doesn't echo our own changes back over SSE. */
export const clientId = crypto.randomUUID?.() ?? Math.random().toString(36).slice(2)

export async function api<T>(path: string, init: RequestInit & { json?: unknown } = {}): Promise<T> {
  const { json, ...rest } = init
  const headers = new Headers(rest.headers)
  headers.set('X-Requested-With', 'fetch')
  headers.set('X-Client-Id', clientId)
  if (json !== undefined) headers.set('Content-Type', 'application/json')
  const res = await fetch(`/api${path}`, {
    ...rest,
    headers,
    body: json !== undefined ? JSON.stringify(json) : rest.body,
    credentials: 'same-origin',
  })
  if (res.status === 204) return undefined as T
  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new ApiError(res.status, data.error ?? res.statusText)
  return data as T
}

// ---------- queries ----------

export const meQuery = queryOptions({
  queryKey: ['me'],
  queryFn: async () => {
    try {
      return await api<User>('/auth/me')
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) return null
      throw e
    }
  },
  staleTime: 5 * 60_000,
})

/** Public runtime config (e.g. the Turnstile site key, when enabled). */
export const configQuery = queryOptions({
  queryKey: ['config'],
  /** devToday (YYYY-MM-DD) is set when the server runs with -dev-day. */
  /** passkeyRpId is the hostname passkeys are bound to, when they are on. */
  queryFn: () => api<{ turnstileSiteKey?: string; devToday?: string; passkeyRpId?: string }>('/config'),
  staleTime: 5 * 60_000,
})

export const securityQuery = queryOptions({
  queryKey: ['security'],
  queryFn: () => api<SecurityInfo>('/me/security'),
})

export const catalogQuery = queryOptions({
  queryKey: ['catalog'],
  queryFn: () => api<Catalog>('/catalog'),
  staleTime: Infinity,
})

/** Query string for signed-out visitors' view (ignored for signed-in users). */
function viewQS(view: CoffeeView | null | undefined): string {
  if (!view) return ''
  return `?today=${view.today}${view.spoilers ? '&spoilers=1' : ''}`
}

export const coffeesQuery = (view: CoffeeView | null = null) =>
  queryOptions({ queryKey: ['coffees', 'list', view], queryFn: () => api<Coffee[]>(`/coffees${viewQS(view)}`) })

export const coffeeQuery = (id: number, view: CoffeeView | null = null) =>
  queryOptions({ queryKey: ['coffees', id, view], queryFn: () => api<CoffeeDetail>(`/coffees/${id}${viewQS(view)}`) })

export const coffeeBrewsQuery = (id: number, view: CoffeeView | null = null) =>
  queryOptions({ queryKey: ['coffees', id, 'brews', view], queryFn: () => api<Brew[]>(`/coffees/${id}/brews${viewQS(view)}`) })

/** Name and origin of a sealed coffee. Not recorded anywhere; just for this visit. */
export const peekQuery = (id: number) =>
  queryOptions({
    queryKey: ['peek', id],
    queryFn: () => api<{ name: string; origin: string }>(`/coffees/${id}/peek`),
    staleTime: Infinity,
  })

export const recipesQuery = queryOptions({ queryKey: ['recipes'], queryFn: () => api<Recipe[]>('/recipes') })
export const grindersQuery = queryOptions({ queryKey: ['grinders'], queryFn: () => api<Grinder[]>('/grinders') })
export const myBrewsQuery = queryOptions({ queryKey: ['brews'], queryFn: () => api<Brew[]>('/brews') })
export const brewQuery = (id: number) =>
  queryOptions({ queryKey: ['brews', id], queryFn: () => api<Brew>(`/brews/${id}`) })

export const myGroupsQuery = queryOptions({ queryKey: ['groups', 'mine'], queryFn: () => api<Group[]>('/groups') })
export const discoverGroupsQuery = (q: string) =>
  queryOptions({
    queryKey: ['groups', 'discover', q],
    queryFn: () => api<Group[]>(`/groups/discover?q=${encodeURIComponent(q)}`),
  })
export const groupQuery = (id: number) =>
  queryOptions({ queryKey: ['groups', id], queryFn: () => api<GroupDetail>(`/groups/${id}`) })

// ---------- mutations ----------

/** A mutation that invalidates the given query key prefixes on success. */
export function useApiMutation<TVars, TResult = unknown>(
  fn: (vars: TVars) => Promise<TResult>,
  invalidate: unknown[][] = [],
) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: fn,
    onSuccess: () => Promise.all(invalidate.map((queryKey) => qc.invalidateQueries({ queryKey }))),
  })
}

/** Everything that depends on brews, opens or favorites. */
export const coffeeKeys = [['coffees'], ['brews'], ['groups']]
