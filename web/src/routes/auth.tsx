import { useState, type FormEvent } from 'react'
import { Link, getRouteApi, useNavigate } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import { api, type User } from '../api'
import { ErrorText, Field } from '../components/ui'

const loginApi = getRouteApi('/login')
const registerApi = getRouteApi('/register')

function AuthShell({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="mx-auto grid max-w-4xl items-center gap-10 py-4 md:grid-cols-2 md:py-12">
      <div>
        <p className="eyebrow mb-3">24 doors · 24 coffees</p>
        <h1 className="display text-5xl sm:text-6xl">
          Rate the <span className="text-accent">calendar</span>, one door at a time.
        </h1>
        <p className="mt-5 max-w-sm text-ink-2">
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
  async function submit(body: unknown) {
    setPending(true)
    setError(null)
    try {
      const user = await api<User>(path, { method: 'POST', json: body })
      qc.clear()
      qc.setQueryData(['me'], user)
      // Only follow same-origin relative redirects.
      const to = redirectTo && redirectTo.startsWith('/') && !redirectTo.startsWith('//') ? redirectTo : '/'
      navigate({ href: to, replace: true })
    } catch (e) {
      setError(e)
    } finally {
      setPending(false)
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
  return (
    <AuthShell title="Create an account">
      <form
        className="space-y-4"
        onSubmit={(e: FormEvent) => {
          e.preventDefault()
          submit({ username, displayName, password })
        }}
      >
        <Field label="Username" hint="3–32 characters: letters, numbers, . _ -">
          <input className="input" autoComplete="username" autoCapitalize="none" value={username} onChange={(e) => setUsername(e.target.value)} required minLength={3} maxLength={32} pattern="[A-Za-z0-9_.\-]+" />
        </Field>
        <Field label="Display name" hint="Shown to other people. Defaults to your username.">
          <input className="input" autoComplete="nickname" value={displayName} onChange={(e) => setDisplayName(e.target.value)} maxLength={50} />
        </Field>
        <Field label="Password" hint="At least 8 characters.">
          <input className="input" type="password" autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)} required minLength={8} maxLength={72} />
        </Field>
        <ErrorText error={error} />
        <button className="btn btn-primary w-full" disabled={pending}>
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
