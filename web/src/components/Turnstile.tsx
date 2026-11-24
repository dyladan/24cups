import { forwardRef, useEffect, useImperativeHandle, useRef } from 'react'

/** Minimal typing for Cloudflare's Turnstile browser API. */
interface TurnstileAPI {
  render(
    el: HTMLElement,
    opts: {
      sitekey: string
      action?: string
      theme?: 'light' | 'dark' | 'auto'
      callback?: (token: string) => void
      'expired-callback'?: () => void
      'error-callback'?: () => void
    },
  ): string
  reset(widgetId: string): void
  remove(widgetId: string): void
}

declare global {
  interface Window {
    turnstile?: TurnstileAPI
  }
}

const SCRIPT = 'https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit'
let loading: Promise<TurnstileAPI> | null = null

function loadTurnstile(): Promise<TurnstileAPI> {
  if (window.turnstile) return Promise.resolve(window.turnstile)
  loading ??= new Promise((resolve, reject) => {
    const s = document.createElement('script')
    s.src = SCRIPT
    s.async = true
    s.defer = true
    s.onload = () => (window.turnstile ? resolve(window.turnstile) : reject(new Error('Turnstile failed to load')))
    s.onerror = () => {
      loading = null
      reject(new Error('Turnstile failed to load'))
    }
    document.head.appendChild(s)
  })
  return loading
}

export interface TurnstileHandle {
  /** Get a fresh token; tokens are single-use, so call after every submit. */
  reset(): void
}

/**
 * Renders a Turnstile widget and reports its token (null when expired,
 * errored or reset). Verification happens on the server, never here.
 */
export const Turnstile = forwardRef<TurnstileHandle, { siteKey: string; action: string; onToken: (t: string | null) => void }>(
  function Turnstile({ siteKey, action, onToken }, ref) {
    const el = useRef<HTMLDivElement>(null)
    const id = useRef<string | null>(null)
    const cb = useRef(onToken)
    cb.current = onToken

    useImperativeHandle(ref, () => ({
      reset() {
        cb.current(null)
        if (id.current && window.turnstile) window.turnstile.reset(id.current)
      },
    }))

    useEffect(() => {
      let cancelled = false
      loadTurnstile()
        .then((ts) => {
          if (cancelled || !el.current) return
          id.current = ts.render(el.current, {
            sitekey: siteKey,
            action,
            theme: document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light',
            callback: (t) => cb.current(t),
            'expired-callback': () => cb.current(null),
            'error-callback': () => cb.current(null),
          })
        })
        .catch(() => cb.current(null))
      return () => {
        cancelled = true
        if (id.current && window.turnstile) window.turnstile.remove(id.current)
        id.current = null
      }
    }, [siteKey, action])

    return <div ref={el} className="min-h-[65px]" />
  },
)
