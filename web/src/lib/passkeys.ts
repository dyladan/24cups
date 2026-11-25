import { startAuthentication, startRegistration, WebAuthnError } from '@simplewebauthn/browser'
import { api, type SecurityInfo, type User } from '../api'

// Passkeys: the server hands out a one-time challenge ("ceremony"), the
// browser's authenticator signs it, and the server checks the signature.

interface Start<T> {
  ceremony: string
  options: { publicKey: T }
}

/** Turn WebAuthn exceptions into messages people can act on. */
function friendly(e: unknown): unknown {
  const name = e instanceof WebAuthnError ? (e.cause as Error | undefined)?.name : e instanceof Error ? e.name : ''
  if (name === 'NotAllowedError' || name === 'AbortError') return new Error('The passkey request was cancelled or timed out.')
  if (e instanceof WebAuthnError && e.code === 'ERROR_AUTHENTICATOR_PREVIOUSLY_REGISTERED')
    return new Error('This device already has a passkey for your account.')
  if (name === 'InvalidStateError') return new Error('This device already has a passkey for your account.')
  if (name === 'SecurityError') return new Error('Passkeys aren’t available on this address.')
  return e
}

async function register<R>(beginPath: string, finishPath: string, body: unknown): Promise<R> {
  const start = await api<Start<Parameters<typeof startRegistration>[0]['optionsJSON']>>(beginPath, { method: 'POST', json: body })
  let credential
  try {
    credential = await startRegistration({ optionsJSON: start.options.publicKey })
  } catch (e) {
    throw friendly(e)
  }
  return api<R>(finishPath, { method: 'POST', json: { ceremony: start.ceremony, credential } })
}

export function passkeySignup(body: { username: string; displayName: string; passkeyName: string; turnstileToken: string }) {
  return register<{ user: User; recoveryCodes: string[] }>('/auth/passkey/register/begin', '/auth/passkey/register/finish', body)
}

export function addPasskey(name: string) {
  return register<SecurityInfo>('/me/passkeys/begin', '/me/passkeys/finish', { name })
}

export async function passkeyLogin(): Promise<User> {
  const start = await api<Start<Parameters<typeof startAuthentication>[0]['optionsJSON']>>('/auth/passkey/login/begin', {
    method: 'POST',
    json: {},
  })
  let credential
  try {
    credential = await startAuthentication({ optionsJSON: start.options.publicKey })
  } catch (e) {
    throw friendly(e)
  }
  return api<User>('/auth/passkey/login/finish', { method: 'POST', json: { ceremony: start.ceremony, credential } })
}

/** Whether this browser can create passkeys at all. */
export const passkeysSupported = typeof window !== 'undefined' && !!window.PublicKeyCredential

/**
 * Whether to offer passkeys on this page: the server has them on, the browser
 * supports them, and we're on the hostname they're bound to (anywhere else,
 * e.g. a LAN or tailnet address, the browser would refuse the ceremony).
 */
export function passkeysUsableHere(rpId: string | undefined): boolean {
  return !!rpId && passkeysSupported && window.location.hostname === rpId
}

/** A starting name for a new passkey, from the device it's made on. */
export function defaultPasskeyName(): string {
  const ua = navigator.userAgent
  if (/iPhone/.test(ua)) return 'iPhone'
  if (/iPad/.test(ua)) return 'iPad'
  if (/Android/.test(ua)) return 'Android'
  if (/Macintosh/.test(ua)) return navigator.maxTouchPoints > 1 ? 'iPad' : 'Mac'
  if (/Windows/.test(ua)) return 'Windows'
  if (/CrOS/.test(ua)) return 'Chromebook'
  if (/Linux/.test(ua)) return 'Linux'
  return 'Passkey'
}
