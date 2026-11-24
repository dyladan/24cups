import { useEffect, useState } from 'react'

/**
 * Light/dark theme with a per-device override. The preference lives in
 * localStorage; index.html applies it before first paint (keep the key and
 * colors in sync with the inline script there).
 */
export type ThemePref = 'system' | 'light' | 'dark'

const KEY = 'advent-theme'
const BG = { light: '#f4eee3', dark: '#141110' }
const media = window.matchMedia('(prefers-color-scheme: dark)')

export function getThemePref(): ThemePref {
  const v = localStorage.getItem(KEY)
  return v === 'light' || v === 'dark' ? v : 'system'
}

function apply(pref: ThemePref) {
  const theme = pref === 'system' ? (media.matches ? 'dark' : 'light') : pref
  document.documentElement.dataset.theme = theme
  document.querySelector('meta[name="theme-color"]')?.setAttribute('content', BG[theme])
}

export function setThemePref(pref: ThemePref) {
  if (pref === 'system') localStorage.removeItem(KEY)
  else localStorage.setItem(KEY, pref)
  apply(pref)
  window.dispatchEvent(new Event('advent-theme'))
}

// Follow OS changes while on "system".
media.addEventListener('change', () => {
  if (getThemePref() === 'system') apply('system')
})

export function useThemePref(): [ThemePref, (p: ThemePref) => void] {
  const [pref, setPref] = useState(getThemePref)
  useEffect(() => {
    const sync = () => setPref(getThemePref())
    window.addEventListener('advent-theme', sync)
    window.addEventListener('storage', sync) // other tabs
    return () => {
      window.removeEventListener('advent-theme', sync)
      window.removeEventListener('storage', sync)
    }
  }, [])
  return [pref, setThemePref]
}
