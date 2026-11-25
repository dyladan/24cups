import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { configQuery, meQuery } from '../api'

/**
 * Signed-out visitors: a per-device "spoilers" switch (community ratings are
 * hidden by default) and their local date, which decides which doors are open.
 */
const KEY = 'advent-anon-spoilers'
const EVENT = 'advent-anon-spoilers'

export function getAnonSpoilers(): boolean {
  try {
    return localStorage.getItem(KEY) === '1'
  } catch {
    return false
  }
}

export function setAnonSpoilers(on: boolean) {
  try {
    if (on) localStorage.setItem(KEY, '1')
    else localStorage.removeItem(KEY)
  } catch {
    /* private mode */
  }
  window.dispatchEvent(new Event(EVENT))
}

export function useAnonSpoilers(): [boolean, (on: boolean) => void] {
  const [on, setOn] = useState(getAnonSpoilers)
  useEffect(() => {
    const sync = () => setOn(getAnonSpoilers())
    window.addEventListener(EVENT, sync)
    window.addEventListener('storage', sync)
    return () => {
      window.removeEventListener(EVENT, sync)
      window.removeEventListener('storage', sync)
    }
  }, [])
  return [on, setAnonSpoilers]
}

/** The visitor's local calendar date, YYYY-MM-DD. */
export function localDate(d = new Date()): string {
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
}

/**
 * Today's date and door number (null outside December). The server can
 * override the date for development (`DAY=3 make dev`); `ready` is false
 * until we know whether it does.
 */
export function useToday(): { date: string; day: number | null; isFuture: (day: number) => boolean; ready: boolean } {
  const { data, isPending } = useQuery(configQuery)
  const date = data?.devToday ?? localDate()
  const [, month, day] = date.split('-').map(Number)
  const today = month === 12 ? day : null
  // Outside December, February–November is before the calendar; January is after.
  const isFuture = (d: number) => (today != null ? d > today : month >= 2)
  return { date, day: today, isFuture, ready: !isPending }
}

// Doors whose Peek a signed-out visitor used on today's door: shown with
// spoilers on the calendar and the coffee page until the page is reloaded.
const peekedToday = new Set<number>()

export function useTodayPeek(coffeeId: number): [boolean, (on: boolean) => void] {
  const [, rerender] = useState(0)
  return [
    peekedToday.has(coffeeId),
    (on: boolean) => {
      if (on) peekedToday.add(coffeeId)
      else peekedToday.delete(coffeeId)
      rerender((n) => n + 1)
    },
  ]
}

/**
 * Query context for coffee requests: today's date (decides which sealed door
 * shows its name, and for signed-out visitors which doors are open) and the
 * signed-out Spoilers switch (ignored for members). Null until ready.
 */
export interface CoffeeView {
  today: string
  spoilers: boolean
}

/**
 * The view to pass to coffee queries. `ready` is false until we know whether
 * the visitor is signed in, so we don't fetch twice.
 */
export function useCoffeeView(): { view: CoffeeView | null; ready: boolean; anon: boolean } {
  const { data: me, isPending } = useQuery(meQuery)
  const [spoilers] = useAnonSpoilers()
  const today = useToday()
  if (isPending || !today.ready) return { view: null, ready: false, anon: !isPending && !me }
  if (me) return { view: { today: today.date, spoilers: false }, ready: true, anon: false }
  return { view: { today: today.date, spoilers }, ready: true, anon: true }
}
