import { queryOptions, useMutation, useQueryClient } from '@tanstack/react-query'

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
}

export interface GroupStats extends Stats {
  groupId: number
  groupName: string
}

export interface CoffeeDetail extends Coffee {
  distribution?: number[]
  groupStats?: GroupStats[]
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
    displayName: string
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

export const catalogQuery = queryOptions({
  queryKey: ['catalog'],
  queryFn: () => api<Catalog>('/catalog'),
  staleTime: Infinity,
})

export const coffeesQuery = queryOptions({
  queryKey: ['coffees'],
  queryFn: () => api<Coffee[]>('/coffees'),
})

export const coffeeQuery = (id: number) =>
  queryOptions({ queryKey: ['coffees', id], queryFn: () => api<CoffeeDetail>(`/coffees/${id}`) })

export const coffeeBrewsQuery = (id: number) =>
  queryOptions({ queryKey: ['coffees', id, 'brews'], queryFn: () => api<Brew[]>(`/coffees/${id}/brews`) })

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
