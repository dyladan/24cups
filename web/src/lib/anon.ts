import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { meQuery } from '../api'

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

/** What a signed-out visitor is looking at; null when signed in. */
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
  if (isPending) return { view: null, ready: false, anon: false }
  if (me) return { view: null, ready: true, anon: false }
  return { view: { today: localDate(), spoilers }, ready: true, anon: true }
}
