import type { RecipeFields } from '../api'

/** Seconds → m:ss */
export function formatTime(s: number | null | undefined): string {
  if (s == null) return '—'
  const m = Math.floor(s / 60)
  const sec = Math.round(s % 60)
  return `${m}:${String(sec).padStart(2, '0')}`
}

/** Accepts "3:15", "3m15s", "195" (seconds) or "3.5" (minutes if it has a decimal point and < 30). */
export function parseTime(input: string): number | null {
  const v = input.trim().toLowerCase()
  if (!v) return null
  let m = v.match(/^(\d+):(\d{1,2})$/)
  if (m) return Number(m[1]) * 60 + Number(m[2])
  m = v.match(/^(?:(\d+)\s*m)?\s*(?:(\d+)\s*s)?$/)
  if (m && (m[1] || m[2])) return Number(m[1] ?? 0) * 60 + Number(m[2] ?? 0)
  const n = Number(v)
  if (!Number.isFinite(n) || n < 0) return NaN
  return Math.round(n)
}

export const cToF = (c: number) => (c * 9) / 5 + 32
export const fToC = (f: number) => ((f - 32) * 5) / 9

export function formatTemp(c: number | null | undefined, unit: 'C' | 'F'): string {
  if (c == null) return '—'
  return unit === 'F' ? `${Math.round(cToF(c))}°F` : `${Math.round(c * 10) / 10}°C`
}

export function round(n: number, digits = 1) {
  const f = 10 ** digits
  return Math.round(n * f) / f
}

export function formatRatio(r: RecipeFields): string {
  const parts = [`1:${round(r.ratio, 2)}`]
  if (r.doseG != null) {
    const out = round(r.doseG * r.ratio, 0)
    parts.push(`${round(r.doseG, 1)}g → ${out}${r.method === 'Espresso' ? 'g' : 'mL'}`)
  }
  return parts.join(' · ')
}

export function formatRating(r: number | null | undefined): string {
  return r == null ? '—' : r.toFixed(2).replace(/0$/, '')
}

export function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}

export function plural(n: number, one: string, many = one + 's') {
  return `${n} ${n === 1 ? one : many}`
}
