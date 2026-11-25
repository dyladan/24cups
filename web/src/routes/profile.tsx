import { useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { KeyRound, LogOut, Pencil, Plus, Trash2, X } from 'lucide-react'
import {
  api,
  catalogQuery,
  grindersQuery,
  meQuery,
  recipesQuery,
  securityQuery,
  useApiMutation,
  type Passkey,
  type SecurityInfo,
  type Grinder,
  type Recipe,
  type SpoilerMode,
  type User,
  type Visibility,
} from '../api'
import { ErrorText, Field, PageHeader, RatioInput, Segmented, Spinner, SuggestInput, TempInput, TimeInput } from '../components/ui'
import { formatDate, formatRatio, formatTemp, formatTime, plural } from '../lib/format'
import { RecoveryCodes } from '../components/RecoveryCodes'
import { addPasskey, defaultPasskeyName, passkeysUsableHere } from '../lib/passkeys'
import { useThemePref, type ThemePref } from '../lib/theme'
import { useLogout } from '../lib/logout'
import { visibilityOptions } from '../lib/visibility'

// ---------- settings ----------

function Settings({ me }: { me: User }) {
  const qc = useQueryClient()
  const [displayName, setDisplayName] = useState(me.displayName)
  const [theme, setTheme] = useThemePref()
  const logout = useLogout()
  const update = useApiMutation((body: Partial<User>) => api<User>('/me', { method: 'PATCH', json: body }), [['coffees'], ['groups']])
  const patch = (body: Partial<User>) => update.mutate(body, { onSuccess: (u) => qc.setQueryData(['me'], u) })

  return (
    <section className="card space-y-5 p-5">
      <h2 className="display text-xl">Settings</h2>
      <form
        className="flex gap-2"
        onSubmit={(e: FormEvent) => {
          e.preventDefault()
          patch({ displayName })
        }}
      >
        <Field label="Display name" className="flex-1">
          <input className="input" value={displayName} onChange={(e) => setDisplayName(e.target.value)} maxLength={50} required />
        </Field>
        <button className="btn btn-ghost self-end" disabled={displayName === me.displayName || update.isPending}>
          Save
        </button>
      </form>
      <p className="-mt-3 text-xs text-ink-3">Signed in as @{me.username}</p>
      <div className="flex items-center justify-between gap-4">
        <span className="label mb-0">Temperature</span>
        <Segmented
          label="Temperature unit"
          value={me.tempUnit}
          onChange={(tempUnit) => patch({ tempUnit })}
          options={[
            { value: 'C', label: '°C' },
            { value: 'F', label: '°F' },
          ]}
        />
      </div>
      <div className="flex items-center justify-between gap-4">
        <span>
          <span className="label mb-0">Appearance</span>
          <span className="block text-xs text-ink-3">Saved on this device</span>
        </span>
        <Segmented<ThemePref>
          label="Appearance"
          value={theme}
          onChange={setTheme}
          options={[
            { value: 'system', label: 'System' },
            { value: 'light', label: 'Light' },
            { value: 'dark', label: 'Dark' },
          ]}
        />
      </div>
      <div>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <span className="label mb-0">Spoilers</span>
          <Segmented<SpoilerMode>
            label="Spoilers"
            value={me.spoilerMode}
            onChange={(spoilerMode) => patch({ spoilerMode })}
            options={[
              { value: 'rated', label: 'Until I rate' },
              { value: 'opened', label: 'Once opened' },
              { value: 'always', label: 'Always' },
            ]}
          />
        </div>
        <p className="mt-1.5 text-xs text-ink-3">
          {me.spoilerMode === 'rated'
            ? 'Blind tasting: tasting notes, community scores and other people’s brews stay hidden until you log a rated brew (or tap Reveal).'
            : me.spoilerMode === 'opened'
              ? 'Tasting notes and community scores appear as soon as you open a door.'
              : 'Also shows community scores on sealed doors. The coffee itself stays hidden until you open it.'}
        </p>
      </div>
      <div>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <span className="label mb-0">New brews visible to</span>
          <Segmented<Visibility>
            label="New brews visible to"
            value={me.defaultVisibility}
            onChange={(defaultVisibility) => patch({ defaultVisibility })}
            options={visibilityOptions}
          />
        </div>
        <p className="mt-1.5 text-xs text-ink-3">The default when you log a brew. You can change it on each brew.</p>
      </div>
      <ErrorText error={update.error} />
      <div className="border-t border-line pt-4">
        <button type="button" className="btn btn-ghost w-full" onClick={() => logout()}>
          <LogOut size={16} /> Sign out
        </button>
      </div>
    </section>
  )
}

// ---------- sign-in & security ----------

function PasskeyRow({ pk, onChange }: { pk: Passkey; onChange: (s: SecurityInfo) => void }) {
  const [editing, setEditing] = useState(false)
  const [name, setName] = useState(pk.name)
  const rename = useApiMutation((n: string) => api<SecurityInfo>(`/me/passkeys/${pk.id}`, { method: 'PATCH', json: { name: n } }))
  const remove = useApiMutation(() => api<SecurityInfo>(`/me/passkeys/${pk.id}`, { method: 'DELETE' }))
  return (
    <li className="py-2">
      {editing ? (
        <form
          className="flex gap-2"
          onSubmit={(e: FormEvent) => {
            e.preventDefault()
            rename.mutate(name, { onSuccess: (s) => (onChange(s), setEditing(false)) })
          }}
        >
          <input className="input flex-1" value={name} onChange={(e) => setName(e.target.value)} maxLength={50} autoFocus aria-label="Passkey name" />
          <button className="btn btn-ghost" disabled={rename.isPending}>
            Save
          </button>
          <button type="button" className="btn btn-ghost" aria-label="Cancel" onClick={() => (setEditing(false), setName(pk.name))}>
            <X size={16} />
          </button>
        </form>
      ) : (
        <div className="flex items-center gap-3">
          <KeyRound size={16} className="shrink-0 text-ink-3" />
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-semibold">{pk.name}</p>
            <p className="text-xs text-ink-3">
              Added {formatDate(pk.createdAt)}
              {pk.lastUsedAt ? ` · last used ${formatDate(pk.lastUsedAt)}` : ''}
            </p>
          </div>
          <button type="button" className="btn btn-ghost px-2" aria-label={`Rename ${pk.name}`} onClick={() => setEditing(true)}>
            <Pencil size={15} />
          </button>
          <button type="button" className="btn btn-ghost px-2" aria-label={`Remove ${pk.name}`} disabled={remove.isPending} onClick={() => remove.mutate(undefined, { onSuccess: onChange })}>
            <Trash2 size={15} />
          </button>
        </div>
      )}
      <ErrorText error={rename.error ?? remove.error} />
    </li>
  )
}

function SignInSecurity({ me }: { me: User }) {
  const qc = useQueryClient()
  const { data: sec } = useQuery(securityQuery)
  const set = (s: SecurityInfo) => qc.setQueryData(['security'], s)
  const refresh = () => qc.invalidateQueries({ queryKey: ['security'] })
  const [codes, setCodes] = useState<string[] | null>(null)
  const [confirmRegen, setConfirmRegen] = useState(false)
  const [cur, setCur] = useState('')
  const [next, setNext] = useState('')

  const add = useApiMutation(() => addPasskey(defaultPasskeyName()))
  const pw = useApiMutation((body: object) => api('/me/password', { method: 'POST', json: body }))
  const dropPw = useApiMutation(() => api<{ recoveryCodes?: string[] }>('/me/password', { method: 'DELETE' }))
  const regen = useApiMutation(() => api<{ recoveryCodes: string[] }>('/me/recovery-codes', { method: 'POST' }))

  if (!sec) return null
  const passkeys = passkeysUsableHere(sec.passkeyRpId)

  if (codes) {
    return (
      <section className="card space-y-4 p-5">
        <h2 className="display text-xl">Save your recovery codes</h2>
        <RecoveryCodes codes={codes} username={me.username} doneLabel="Done" onDone={() => (setCodes(null), refresh())} />
      </section>
    )
  }

  return (
    <section className="card space-y-5 p-5">
      <h2 className="display text-xl">Sign-in</h2>

      {passkeys && (
        <div className="space-y-2">
          <span className="label mb-0">Passkeys</span>
          {sec.passkeys.length > 0 ? (
            <ul className="divide-y divide-line">
              {sec.passkeys.map((pk) => (
                <PasskeyRow key={pk.id} pk={pk} onChange={set} />
              ))}
            </ul>
          ) : (
            <p className="text-xs text-ink-3">
              Sign in with your fingerprint, face or device PIN instead of a password. Your device keeps the key; this site only ever
              sees a public key.
            </p>
          )}
          <button type="button" className="btn btn-ghost w-full" disabled={add.isPending} onClick={() => add.mutate(undefined, { onSuccess: set })}>
            <Plus size={16} /> {add.isPending ? 'Waiting for your device…' : 'Add a passkey'}
          </button>
          <ErrorText error={add.error} />
        </div>
      )}

      <div className="space-y-2 border-t border-line pt-4">
        <span className="label mb-0">Password</span>
        <form
          className="space-y-3"
          onSubmit={(e: FormEvent) => {
            e.preventDefault()
            pw.mutate({ currentPassword: cur, newPassword: next }, { onSuccess: () => (setCur(''), setNext(''), refresh()) })
          }}
        >
          {sec.hasPassword ? (
            <input className="input" type="password" autoComplete="current-password" placeholder="Current password" value={cur} onChange={(e) => setCur(e.target.value)} required />
          ) : (
            <p className="text-xs text-ink-3">You sign in with passkeys only. You can add a password as a fallback.</p>
          )}
          <input className="input" type="password" autoComplete="new-password" placeholder="New password (8+ characters)" value={next} onChange={(e) => setNext(e.target.value)} minLength={8} maxLength={72} required />
          <ErrorText error={pw.error} />
          {pw.isSuccess && <p className="text-sm text-green-700 dark:text-green-400">Password saved. Other devices were signed out.</p>}
          <button className="btn btn-ghost w-full" disabled={pw.isPending}>
            {sec.hasPassword ? 'Change password' : 'Add a password'}
          </button>
        </form>
        {passkeys && sec.hasPassword && sec.passkeys.length > 0 && (
          <>
            <button
              type="button"
              className="btn btn-ghost w-full"
              disabled={dropPw.isPending}
              onClick={() =>
                dropPw.mutate(undefined, {
                  onSuccess: (r) => (r.recoveryCodes ? setCodes(r.recoveryCodes) : refresh()),
                })
              }
            >
              Remove password, use passkeys only
            </button>
            <ErrorText error={dropPw.error} />
          </>
        )}
      </div>

      {(sec.recoveryCodesLeft > 0 || sec.passkeys.length > 0) && (
        <div className="space-y-2 border-t border-line pt-4">
          <span className="label mb-0">Recovery codes</span>
          <p className="text-xs text-ink-3">
            {sec.recoveryCodesLeft > 0
              ? `${plural(sec.recoveryCodesLeft, 'unused code')} left. Each one signs you in once if you lose your passkeys.`
              : 'You have no recovery codes. Make some in case you lose your passkeys.'}
          </p>
          {confirmRegen ? (
            <div className="flex gap-2">
              <button
                type="button"
                className="btn btn-primary flex-1"
                disabled={regen.isPending}
                onClick={() => regen.mutate(undefined, { onSuccess: (r) => (setConfirmRegen(false), setCodes(r.recoveryCodes)) })}
              >
                Replace my old codes
              </button>
              <button type="button" className="btn btn-ghost" onClick={() => setConfirmRegen(false)}>
                Cancel
              </button>
            </div>
          ) : (
            <button
              type="button"
              className="btn btn-ghost w-full"
              onClick={() => (sec.recoveryCodesLeft > 0 ? setConfirmRegen(true) : regen.mutate(undefined, { onSuccess: (r) => setCodes(r.recoveryCodes) }))}
            >
              {sec.recoveryCodesLeft > 0 ? 'Make new codes' : 'Make recovery codes'}
            </button>
          )}
          <ErrorText error={regen.error} />
        </div>
      )}
    </section>
  )
}

// ---------- recipes ----------

type RecipeDraft = Omit<Recipe, 'id' | 'ratio'> & { ratio: number | null }
const blankRecipe: RecipeDraft = { name: '', method: '', doseG: null, ratio: null, tempC: null, targetTimeS: null, notes: '' }

function RecipeForm({ initial, id, onDone, unit }: { initial: RecipeDraft; id?: number; onDone: () => void; unit: 'C' | 'F' }) {
  const { data: catalog } = useQuery(catalogQuery)
  const [d, setD] = useState(initial)
  const set = (p: Partial<RecipeDraft>) => setD((x) => ({ ...x, ...p }))
  const save = useApiMutation(
    (body: object) => (id ? api(`/recipes/${id}`, { method: 'PUT', json: body }) : api('/recipes', { method: 'POST', json: body })),
    [['recipes']],
  )
  return (
    <form
      className="space-y-4 rounded-xl border border-ink p-4"
      onSubmit={(e: FormEvent) => {
        e.preventDefault()
        save.mutate(d, { onSuccess: onDone })
      }}
    >
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Preset name">
          <input className="input" value={d.name} onChange={(e) => set({ name: e.target.value })} placeholder="Morning V60" required maxLength={80} />
        </Field>
        <Field label="Equipment">
          <SuggestInput id="recipe-methods" value={d.method} onChange={(method) => set({ method })} options={catalog?.methods ?? []} placeholder="V60" required />
        </Field>
      </div>
      <RatioInput doseG={d.doseG} ratio={d.ratio} method={d.method} onChange={(v) => set(v)} />
      <div className="grid grid-cols-2 gap-3">
        <Field label="Water temp">
          <TempInput tempC={d.tempC} unit={unit} onChange={(tempC) => set({ tempC })} />
        </Field>
        <Field label="Target time">
          <TimeInput seconds={d.targetTimeS} onChange={(targetTimeS) => set({ targetTimeS })} />
        </Field>
      </div>
      <Field label="Notes">
        <textarea className="input min-h-16" value={d.notes} onChange={(e) => set({ notes: e.target.value })} placeholder="Pour schedule, filter, water…" maxLength={2000} />
      </Field>
      <ErrorText error={save.error} />
      <div className="flex gap-2">
        <button className="btn btn-primary" disabled={save.isPending || d.ratio == null}>
          {id ? 'Save preset' : 'Add preset'}
        </button>
        <button type="button" className="btn btn-ghost" onClick={onDone}>
          Cancel
        </button>
      </div>
    </form>
  )
}

function Recipes({ unit }: { unit: 'C' | 'F' }) {
  const { data, isPending } = useQuery(recipesQuery)
  const [editing, setEditing] = useState<number | 'new' | null>(null)
  const del = useApiMutation((id: number) => api(`/recipes/${id}`, { method: 'DELETE' }), [['recipes']])
  return (
    <section id="recipes" className="card scroll-mt-24 space-y-4 p-5">
      <div className="flex items-center justify-between">
        <h2 className="display text-xl">Recipe presets</h2>
        {editing !== 'new' && (
          <button className="btn btn-ghost !py-1.5" onClick={() => setEditing('new')}>
            <Plus size={15} /> Add
          </button>
        )}
      </div>
      {editing === 'new' && <RecipeForm initial={blankRecipe} unit={unit} onDone={() => setEditing(null)} />}
      {isPending ? (
        <Spinner />
      ) : data?.length ? (
        <ul className="divide-y divide-line">
          {data.map((r) =>
            editing === r.id ? (
              <li key={r.id} className="py-3">
                <RecipeForm initial={r} id={r.id} unit={unit} onDone={() => setEditing(null)} />
              </li>
            ) : (
              <li key={r.id} className="flex items-start gap-3 py-3">
                <div className="min-w-0 flex-1">
                  <p className="font-semibold">{r.name}</p>
                  <p className="text-sm text-ink-2">
                    {r.method} · {formatRatio(r)} · {formatTemp(r.tempC, unit)} · {formatTime(r.targetTimeS)}
                  </p>
                  {r.notes && <p className="mt-1 line-clamp-2 text-xs text-ink-3">{r.notes}</p>}
                </div>
                <button className="p-1.5 text-ink-3 hover:text-ink" onClick={() => setEditing(r.id)} aria-label={`Edit ${r.name}`}>
                  <Pencil size={15} />
                </button>
                <button className="p-1.5 text-ink-3 hover:text-red-600" onClick={() => del.mutate(r.id)} aria-label={`Delete ${r.name}`}>
                  <Trash2 size={15} />
                </button>
              </li>
            ),
          )}
        </ul>
      ) : (
        editing !== 'new' && <p className="text-sm text-ink-3">Save the recipes you brew often, then pick one when logging a brew.</p>
      )}
    </section>
  )
}

// ---------- grinders ----------

type GrinderDraft = Omit<Grinder, 'id'>
const blankGrinder: GrinderDraft = { model: '', nickname: '', burrsets: [], notes: '' }

function GrinderForm({ initial, id, onDone }: { initial: GrinderDraft; id?: number; onDone: () => void }) {
  const { data: catalog } = useQuery(catalogQuery)
  const [d, setD] = useState(initial)
  const [burr, setBurr] = useState('')
  const set = (p: Partial<GrinderDraft>) => setD((x) => ({ ...x, ...p }))
  const save = useApiMutation(
    (body: object) => (id ? api(`/grinders/${id}`, { method: 'PUT', json: body }) : api('/grinders', { method: 'POST', json: body })),
    [['grinders']],
  )
  const catalogEntry = catalog?.grinders.find((g) => g.model === d.model)
  function addBurr(b: string) {
    b = b.trim()
    if (b && !d.burrsets.includes(b)) set({ burrsets: [...d.burrsets, b] })
    setBurr('')
  }
  return (
    <form
      className="space-y-4 rounded-xl border border-ink p-4"
      onSubmit={(e: FormEvent) => {
        e.preventDefault()
        const burrsets = burr.trim() && !d.burrsets.includes(burr.trim()) ? [...d.burrsets, burr.trim()] : d.burrsets
        save.mutate({ ...d, burrsets }, { onSuccess: onDone })
      }}
    >
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Grinder" hint="Pick from the list or type your own.">
          <SuggestInput id="grinder-models" value={d.model} onChange={(model) => set({ model })} options={catalog?.grinders.map((g) => g.model) ?? []} placeholder="Comandante C40" required />
        </Field>
        <Field label="Nickname (optional)">
          <input className="input" value={d.nickname} onChange={(e) => set({ nickname: e.target.value })} placeholder="Work grinder" maxLength={50} />
        </Field>
      </div>
      <div>
        <span className="label">Burrsets</span>
        <div className="mb-2 flex flex-wrap gap-1.5">
          {d.burrsets.map((b, i) => (
            <span key={b} className="chip !py-1 !text-ink">
              {i === 0 && <span className="text-[0.6rem] font-bold uppercase text-accent">Default</span>}
              {b}
              <button type="button" onClick={() => set({ burrsets: d.burrsets.filter((x) => x !== b) })} aria-label={`Remove ${b}`} className="text-ink-3 hover:text-ink">
                <X size={12} />
              </button>
            </span>
          ))}
          {catalogEntry && !d.burrsets.includes(catalogEntry.stockBurrs) && (
            <button type="button" className="chip border-dashed hover:border-ink" onClick={() => addBurr(catalogEntry.stockBurrs)}>
              <Plus size={11} /> {catalogEntry.stockBurrs}
            </button>
          )}
        </div>
        <div className="flex gap-2">
          <SuggestInput id="burrsets" value={burr} onChange={setBurr} options={catalog?.burrsets ?? []} placeholder="SSP 64mm Multipurpose" />
          <button type="button" className="btn btn-ghost" onClick={() => addBurr(burr)} disabled={!burr.trim()}>
            Add
          </button>
        </div>
        <p className="mt-1 text-xs text-ink-3">Swap burrs? Add each set. The first one is the default when logging a brew.</p>
      </div>
      <ErrorText error={save.error} />
      <div className="flex gap-2">
        <button className="btn btn-primary" disabled={save.isPending}>
          {id ? 'Save grinder' : 'Add grinder'}
        </button>
        <button type="button" className="btn btn-ghost" onClick={onDone}>
          Cancel
        </button>
      </div>
    </form>
  )
}

function Grinders() {
  const { data, isPending } = useQuery(grindersQuery)
  const [editing, setEditing] = useState<number | 'new' | null>(null)
  const del = useApiMutation((id: number) => api(`/grinders/${id}`, { method: 'DELETE' }), [['grinders']])
  return (
    <section id="grinders" className="card scroll-mt-24 space-y-4 p-5">
      <div className="flex items-center justify-between">
        <h2 className="display text-xl">Grinders</h2>
        {editing !== 'new' && (
          <button className="btn btn-ghost !py-1.5" onClick={() => setEditing('new')}>
            <Plus size={15} /> Add
          </button>
        )}
      </div>
      {editing === 'new' && <GrinderForm initial={blankGrinder} onDone={() => setEditing(null)} />}
      {isPending ? (
        <Spinner />
      ) : data?.length ? (
        <ul className="divide-y divide-line">
          {data.map((g) =>
            editing === g.id ? (
              <li key={g.id} className="py-3">
                <GrinderForm initial={g} id={g.id} onDone={() => setEditing(null)} />
              </li>
            ) : (
              <li key={g.id} className="flex items-start gap-3 py-3">
                <div className="min-w-0 flex-1">
                  <p className="font-semibold">
                    {g.nickname || g.model}
                    {g.nickname && <span className="font-normal text-ink-2"> · {g.model}</span>}
                  </p>
                  <div className="mt-1 flex flex-wrap gap-1">
                    {g.burrsets.length ? g.burrsets.map((b) => <span key={b} className="chip">{b}</span>) : <span className="text-xs text-ink-3">No burrsets</span>}
                  </div>
                </div>
                <button className="p-1.5 text-ink-3 hover:text-ink" onClick={() => setEditing(g.id)} aria-label={`Edit ${g.model}`}>
                  <Pencil size={15} />
                </button>
                <button className="p-1.5 text-ink-3 hover:text-red-600" onClick={() => del.mutate(g.id)} aria-label={`Delete ${g.model}`}>
                  <Trash2 size={15} />
                </button>
              </li>
            ),
          )}
        </ul>
      ) : (
        editing !== 'new' && <p className="text-sm text-ink-3">Add the grinders you use so brews record your setup.</p>
      )}
    </section>
  )
}

export function ProfilePage() {
  const { data: me } = useQuery(meQuery)
  if (!me) return <Spinner />
  return (
    <>
      <PageHeader eyebrow={`@${me.username}`} title="Profile" />
      <div className="grid gap-6 lg:grid-cols-[1fr_380px]">
        <div className="space-y-6">
          <Recipes unit={me.tempUnit} />
          <Grinders />
        </div>
        <div className="space-y-6">
          <Settings me={me} />
          <SignInSecurity me={me} />
        </div>
      </div>
    </>
  )
}
