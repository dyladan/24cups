import { useRef, useState, type FormEvent } from 'react'
import { Link, getRouteApi, useNavigate } from '@tanstack/react-router'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { KeyRound } from 'lucide-react'
import { api, configQuery, type User } from '../api'
import { RecoveryCodes } from '../components/RecoveryCodes'
import { Turnstile, type TurnstileHandle } from '../components/Turnstile'
import { Blossom, ErrorText, Field, Segmented } from '../components/ui'
import { defaultPasskeyName, passkeyLogin, passkeySignup, passkeysSupported } from '../lib/passkeys'

const loginApi = getRouteApi('/login')
const registerApi = getRouteApi('/register')

function AuthShell({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="mx-auto grid max-w-4xl items-center gap-10 py-4 md:grid-cols-2 md:py-12">
      <div className="relative overflow-hidden rounded-3xl bg-door p-8 text-door-ink sm:p-10">
        <span className="pointer-events-none absolute inset-3 rounded-2xl border border-door-ink/25" />
        <Blossom size={150} className="pointer-events-none absolute -right-8 -bottom-8 opacity-10" />
        <p className="relative mb-4 text-[0.68rem] font-semibold uppercase tracking-[0.3em] opacity-80">24 doors · 24 coffees</p>
        <h1 className="display relative !text-door-ink text-5xl sm:text-6xl">Rate the calendar, one door at a time.</h1>
        <p className="relative mt-5 max-w-sm text-[#f6e9d8]">
          Log every brew, dial in with your group, and see how the community scores each coffee — without spoiling the doors you
          haven’t opened yet.
        </p>
      </div>
      <div className="card p-6 sm:p-8">
        <h2 className="display mb-6 text-2xl">{title}</h2>
        {children}
      </div>
    </div>
  )
}

/** Runs a sign-in or sign-up request, then switches to the new account. */
function useAuthFlow(redirectTo?: string) {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const [error, setError] = useState<unknown>(null)
  const [pending, setPending] = useState(false)

  /** Store the signed-in user and leave the page. */
  function finish(user: User, to?: string) {
    // Drop the previous account's data but keep the ['me'] query itself:
    // qc.clear() would orphan the header's observer, hiding the nav until a reload.
    qc.removeQueries({ predicate: (q) => q.queryKey[0] !== 'me' })
    qc.setQueryData(['me'], user)
    // Only follow same-origin relative redirects.
    const target = to ?? (redirectTo && redirectTo.startsWith('/') && !redirectTo.startsWith('//') ? redirectTo : '/')
    navigate({ href: target, replace: true })
  }

  /** Run fn; on success hand its result to done (default: finish). */
  async function run<T>(fn: () => Promise<T>, done: (r: T) => void, after?: () => void) {
    setPending(true)
    setError(null)
    try {
      done(await fn())
    } catch (e) {
      setError(e)
    } finally {
      setPending(false)
      after?.()
    }
  }
  return { run, finish, error, pending }
}

function Divider({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex items-center gap-3 text-xs uppercase tracking-widest text-ink-3">
      <span className="h-px flex-1 bg-line" />
      {children}
      <span className="h-px flex-1 bg-line" />
    </div>
  )
}

function usePasskeysAvailable() {
  const { data: config } = useQuery(configQuery)
  return !!config?.passkeys && passkeysSupported
}

export function LoginPage() {
  const { redirect } = loginApi.useSearch()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [recovering, setRecovering] = useState(false)
  const passkeys = usePasskeysAvailable()
  const { run, finish, error, pending } = useAuthFlow(redirect)
  return (
    <AuthShell title={recovering ? 'Use a recovery code' : 'Sign in'}>
      <div className="space-y-4">
        {passkeys && !recovering && (
          <>
            <button type="button" className="btn btn-primary w-full" disabled={pending} onClick={() => run(passkeyLogin, (u) => finish(u))}>
              <KeyRound size={16} /> Sign in with a passkey
            </button>
            <Divider>or use your password</Divider>
          </>
        )}
        <form
          className="space-y-4"
          onSubmit={(e: FormEvent) => {
            e.preventDefault()
            if (recovering) {
              // Recovery means a lost passkey: go add a new one.
              run(() => api<User>('/auth/recover', { method: 'POST', json: { username, code } }), (u) => finish(u, '/profile'))
            } else {
              run(() => api<User>('/auth/login', { method: 'POST', json: { username, password } }), (u) => finish(u))
            }
          }}
        >
          <Field label="Username">
            <input className="input" autoComplete="username" autoCapitalize="none" value={username} onChange={(e) => setUsername(e.target.value)} required />
          </Field>
          {recovering ? (
            <Field label="Recovery code" hint="One of the codes you saved when you set up your passkey. Each code works once.">
              <input className="input font-mono" autoComplete="one-time-code" autoCapitalize="characters" spellCheck={false} placeholder="XXXX-XXXX-XXXX" value={code} onChange={(e) => setCode(e.target.value)} required />
            </Field>
          ) : (
            <Field label="Password">
              <input className="input" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} required />
            </Field>
          )}
          <ErrorText error={error} />
          <button className={`btn w-full ${passkeys && !recovering ? 'btn-ghost' : 'btn-primary'}`} disabled={pending}>
            {pending ? 'Signing in…' : 'Sign in'}
          </button>
          {passkeys && (
            <p className="text-center text-sm text-ink-2">
              <button type="button" className="underline underline-offset-4" onClick={() => setRecovering(!recovering)}>
                {recovering ? 'Back to sign in' : 'Lost your passkey? Use a recovery code'}
              </button>
            </p>
          )}
          <p className="text-center text-sm text-ink-2">
            New here?{' '}
            <Link to="/register" search={{ redirect }} className="font-semibold text-ink underline underline-offset-4">
              Create an account
            </Link>
          </p>
        </form>
      </div>
    </AuthShell>
  )
}

export function RegisterPage() {
  const { redirect } = registerApi.useSearch()
  const [username, setUsername] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [password, setPassword] = useState('')
  const passkeys = usePasskeysAvailable()
  const [method, setMethod] = useState<'passkey' | 'password'>('passkey')
  const usePasskey = passkeys && method === 'passkey'
  const { run, finish, error, pending } = useAuthFlow(redirect)
  const { data: config } = useQuery(configQuery)
  const siteKey = config?.turnstileSiteKey
  const [token, setToken] = useState<string | null>(null)
  const turnstile = useRef<TurnstileHandle>(null)
  const [created, setCreated] = useState<{ user: User; recoveryCodes: string[] } | null>(null)

  if (created) {
    return (
      <AuthShell title="Save your recovery codes">
        <RecoveryCodes codes={created.recoveryCodes} username={created.user.username} onDone={() => finish(created.user)} />
      </AuthShell>
    )
  }
  return (
    <AuthShell title="Create an account">
      <form
        className="space-y-4"
        onSubmit={(e: FormEvent) => {
          e.preventDefault()
          // Tokens are single-use: get a fresh one after every attempt.
          const reset = () => turnstile.current?.reset()
          const turnstileToken = token ?? ''
          if (usePasskey) {
            run(() => passkeySignup({ username, displayName, passkeyName: defaultPasskeyName(), turnstileToken }), setCreated, reset)
          } else {
            run(() => api<User>('/auth/register', { method: 'POST', json: { username, displayName, password, turnstileToken } }), (u) => finish(u), reset)
          }
        }}
      >
        <Field label="Username" hint="3–32 characters: letters, numbers, . _ -">
          <input className="input" autoComplete="username" autoCapitalize="none" value={username} onChange={(e) => setUsername(e.target.value)} required minLength={3} maxLength={32} pattern="[A-Za-z0-9_.\-]+" />
        </Field>
        <Field label="Display name (optional)" hint="What other people see. Leave blank to use your username; you can change it later in your profile.">
          <input className="input" autoComplete="nickname" value={displayName} onChange={(e) => setDisplayName(e.target.value)} maxLength={50} placeholder={username.trim() || 'Same as your username'} />
        </Field>
        {passkeys && (
          <div className="space-y-1.5">
            <div className="flex items-center justify-between gap-3">
              <span className="label mb-0">Sign in with</span>
              <Segmented
                label="Sign in with"
                value={method}
                onChange={setMethod}
                options={[
                  { value: 'passkey', label: 'Passkey' },
                  { value: 'password', label: 'Password' },
                ]}
              />
            </div>
            {usePasskey && (
              <p className="text-xs text-ink-3">
                Your device or password manager keeps the key; this site never sees a password or anything it could reuse.
              </p>
            )}
          </div>
        )}
        {!usePasskey && (
          <Field label="Password" hint="At least 8 characters.">
            <input className="input" type="password" autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)} required minLength={8} maxLength={72} />
          </Field>
        )}
        {siteKey && <Turnstile ref={turnstile} siteKey={siteKey} action="signup" onToken={setToken} />}
        <ErrorText error={error} />
        <button className="btn btn-primary w-full" disabled={pending || (!!siteKey && !token)}>
          {usePasskey && <KeyRound size={16} />}
          {pending ? 'Creating…' : usePasskey ? 'Create account with a passkey' : 'Create account'}
        </button>
        <p className="text-center text-sm text-ink-2">
          Already have one?{' '}
          <Link to="/login" search={{ redirect }} className="font-semibold text-ink underline underline-offset-4">
            Sign in
          </Link>
        </p>
      </form>
    </AuthShell>
  )
}
