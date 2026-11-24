import { useRef, useState, type FormEvent } from 'react'
import { Link, getRouteApi, useNavigate } from '@tanstack/react-router'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, configQuery, type User } from '../api'
import { Turnstile, type TurnstileHandle } from '../components/Turnstile'
import { Blossom, ErrorText, Field } from '../components/ui'

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

function useAuthSubmit(path: string, redirectTo?: string) {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const [error, setError] = useState<unknown>(null)
  const [pending, setPending] = useState(false)
  async function submit(body: unknown, after?: () => void) {
    setPending(true)
    setError(null)
    try {
      const user = await api<User>(path, { method: 'POST', json: body })
      // Drop the previous account's data but keep the ['me'] query itself:
      // qc.clear() would orphan the header's observer, hiding the nav until a reload.
      qc.removeQueries({ predicate: (q) => q.queryKey[0] !== 'me' })
      qc.setQueryData(['me'], user)
      // Only follow same-origin relative redirects.
      const to = redirectTo && redirectTo.startsWith('/') && !redirectTo.startsWith('//') ? redirectTo : '/'
      navigate({ href: to, replace: true })
    } catch (e) {
      setError(e)
    } finally {
      setPending(false)
      after?.()
    }
  }
  return { submit, error, pending }
}

export function LoginPage() {
  const { redirect } = loginApi.useSearch()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const { submit, error, pending } = useAuthSubmit('/auth/login', redirect)
  return (
    <AuthShell title="Sign in">
      <form
        className="space-y-4"
        onSubmit={(e: FormEvent) => {
          e.preventDefault()
          submit({ username, password })
        }}
      >
        <Field label="Username">
          <input className="input" autoComplete="username" autoCapitalize="none" value={username} onChange={(e) => setUsername(e.target.value)} required />
        </Field>
        <Field label="Password">
          <input className="input" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} required />
        </Field>
        <ErrorText error={error} />
        <button className="btn btn-primary w-full" disabled={pending}>
          {pending ? 'Signing in…' : 'Sign in'}
        </button>
        <p className="text-center text-sm text-ink-2">
          New here?{' '}
          <Link to="/register" search={{ redirect }} className="font-semibold text-ink underline underline-offset-4">
            Create an account
          </Link>
        </p>
      </form>
    </AuthShell>
  )
}

export function RegisterPage() {
  const { redirect } = registerApi.useSearch()
  const [username, setUsername] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [password, setPassword] = useState('')
  const { submit, error, pending } = useAuthSubmit('/auth/register', redirect)
  const { data: config } = useQuery(configQuery)
  const siteKey = config?.turnstileSiteKey
  const [token, setToken] = useState<string | null>(null)
  const turnstile = useRef<TurnstileHandle>(null)
  return (
    <AuthShell title="Create an account">
      <form
        className="space-y-4"
        onSubmit={(e: FormEvent) => {
          e.preventDefault()
          // Tokens are single-use: get a fresh one after every attempt.
          submit({ username, displayName, password, turnstileToken: token ?? '' }, () => turnstile.current?.reset())
        }}
      >
        <Field label="Username" hint="3–32 characters: letters, numbers, . _ -">
          <input className="input" autoComplete="username" autoCapitalize="none" value={username} onChange={(e) => setUsername(e.target.value)} required minLength={3} maxLength={32} pattern="[A-Za-z0-9_.\-]+" />
        </Field>
        <Field label="Display name (optional)" hint="What other people see. Leave blank to use your username; you can change it later in your profile.">
          <input className="input" autoComplete="nickname" value={displayName} onChange={(e) => setDisplayName(e.target.value)} maxLength={50} placeholder={username.trim() || 'Same as your username'} />
        </Field>
        <Field label="Password" hint="At least 8 characters.">
          <input className="input" type="password" autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)} required minLength={8} maxLength={72} />
        </Field>
        {siteKey && <Turnstile ref={turnstile} siteKey={siteKey} action="signup" onToken={setToken} />}
        <ErrorText error={error} />
        <button className="btn btn-primary w-full" disabled={pending || (!!siteKey && !token)}>
          {pending ? 'Creating…' : 'Create account'}
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
