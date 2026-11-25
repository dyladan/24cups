import { useMemo, useState, type FormEvent } from 'react'
import { Link, getRouteApi, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { ArrowLeft, Trash2 } from 'lucide-react'
import {
  api,
  brewQuery,
  catalogQuery,
  coffeeKeys,
  coffeesQuery,
  grindersQuery,
  meQuery,
  recipesQuery,
  useApiMutation,
  type Brew,
  type BrewInput,
} from '../api'
import { ErrorText, Field, PageHeader, RatioInput, Spinner, StarInput, SuggestInput, TempInput, TimeInput, Toggle } from '../components/ui'
import { formatRatio, formatTemp, formatTime } from '../lib/format'
import { FlavorPicker } from '../components/Flavors'

const newRoute = getRouteApi('/app/brews/new')
const editRoute = getRouteApi('/app/brews/$brewId/edit')

type Draft = Omit<BrewInput, 'ratio'> & { ratio: number | null }

const emptyDraft = (coffeeId: number | undefined): Draft => ({
  coffeeId: coffeeId ?? 0,
  recipeId: null,
  method: '',
  doseG: null,
  ratio: null,
  tempC: null,
  targetTimeS: null,
  actualTimeS: null,
  grinderId: null,
  burrset: '',
  grindSetting: '',
  rating: null,
  notes: '',
  descriptors: [],
  isPublic: true,
  excludeFromGlobal: false,
})

function fromBrew(b: Brew): Draft {
  return {
    coffeeId: b.coffeeId,
    recipeId: b.recipeId,
    method: b.method,
    doseG: b.doseG,
    ratio: b.ratio,
    tempC: b.tempC,
    targetTimeS: b.targetTimeS,
    actualTimeS: b.actualTimeS,
    grinderId: b.grinderId,
    burrset: b.burrset,
    grindSetting: b.grindSetting,
    rating: b.rating,
    notes: b.notes,
    descriptors: b.descriptors ?? [],
    isPublic: b.isPublic,
    excludeFromGlobal: b.excludeFromGlobal,
    brewedAt: b.brewedAt,
  }
}

function BrewForm({ initial, brewId }: { initial: Draft; brewId?: number }) {
  const navigate = useNavigate()
  const { data: me } = useQuery(meQuery)
  const { data: coffees } = useQuery(coffeesQuery())
  const { data: recipes } = useQuery(recipesQuery)
  const { data: grinders } = useQuery(grindersQuery)
  const { data: catalog } = useQuery(catalogQuery)
  const [d, setD] = useState<Draft>(initial)
  // Bumped when a preset is applied so the uncontrolled-text inputs re-read their values.
  const [presetKey, setPresetKey] = useState(0)
  const unit = me?.tempUnit ?? 'C'
  const set = (patch: Partial<Draft>) => setD((prev) => ({ ...prev, ...patch }))

  const grinder = grinders?.find((g) => g.id === d.grinderId)
  const coffee = coffees?.find((c) => c.id === d.coffeeId)

  const save = useApiMutation(
    (body: BrewInput) =>
      brewId ? api<Brew>(`/brews/${brewId}`, { method: 'PUT', json: body }) : api<Brew>('/brews', { method: 'POST', json: body }),
    coffeeKeys,
  )
  const del = useApiMutation(() => api(`/brews/${brewId}`, { method: 'DELETE' }), coffeeKeys)

  const sortedCoffees = useMemo(
    () => [...(coffees ?? [])].sort((a, b) => Number(b.opened) - Number(a.opened) || a.day - b.day),
    [coffees],
  )

  function applyRecipe(id: number | null) {
    const r = recipes?.find((x) => x.id === id)
    if (!r) {
      set({ recipeId: null })
      return
    }
    set({ recipeId: r.id, method: r.method, doseG: r.doseG, ratio: r.ratio, tempC: r.tempC, targetTimeS: r.targetTimeS })
    setPresetKey((k) => k + 1)
  }

  function submit(e: FormEvent) {
    e.preventDefault()
    if (!d.coffeeId || d.ratio == null) return
    save.mutate({ ...d, ratio: d.ratio }, {
      onSuccess: (b) => navigate({ to: '/coffee/$coffeeId', params: { coffeeId: b.coffeeId } }),
    })
  }

  const missing = !d.coffeeId ? 'Pick a coffee.' : !d.method ? 'Pick your equipment.' : d.ratio == null ? 'Enter a brew ratio.' : null

  return (
    <form onSubmit={submit} className="grid gap-6 lg:grid-cols-[1fr_320px] lg:gap-10">
      <div className="space-y-6">
        <section className="card space-y-4 p-5">
          <h2 className="display text-xl">Coffee</h2>
          <Field label="Day">
            <select className="input" value={d.coffeeId || ''} onChange={(e) => set({ coffeeId: Number(e.target.value) })} required>
              <option value="" disabled>
                Choose a coffee…
              </option>
              {sortedCoffees.map((c) => (
                <option key={c.id} value={c.id}>
                  Day {c.day} — {c.opened ? c.info?.name : 'unopened (will open it)'}
                </option>
              ))}
            </select>
          </Field>
          {coffee && !coffee.opened && <p className="text-xs text-ink-3">Saving this brew will open door {coffee.day}.</p>}
        </section>

        <section className="card space-y-4 p-5">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <h2 className="display text-xl">Recipe</h2>
            {recipes && recipes.length > 0 ? (
              <select
                className="input !w-auto !py-1.5 text-sm"
                value={d.recipeId ?? ''}
                onChange={(e) => applyRecipe(e.target.value ? Number(e.target.value) : null)}
                aria-label="Fill from a preset"
              >
                <option value="">Fill from preset…</option>
                {recipes.map((r) => (
                  <option key={r.id} value={r.id}>
                    {r.name}
                  </option>
                ))}
              </select>
            ) : (
              <Link to="/profile" hash="recipes" className="text-xs font-semibold text-ink-2 underline underline-offset-4">
                Save presets in your profile
              </Link>
            )}
          </div>
          <div key={presetKey} className="space-y-4">
            <Field label="Equipment">
              <SuggestInput id="methods" value={d.method} onChange={(v) => set({ method: v })} options={catalog?.methods ?? []} placeholder="V60, Espresso, AeroPress…" required />
            </Field>
            <RatioInput doseG={d.doseG} ratio={d.ratio} method={d.method} onChange={(v) => set(v)} />
            <div className="grid grid-cols-2 gap-3">
              <Field label="Water temp">
                <TempInput tempC={d.tempC} unit={unit} onChange={(tempC) => set({ tempC })} />
              </Field>
              <Field label="Target time">
                <TimeInput seconds={d.targetTimeS} onChange={(targetTimeS) => set({ targetTimeS })} />
              </Field>
            </div>
          </div>
        </section>

        <section className="card space-y-4 p-5">
          <h2 className="display text-xl">The brew</h2>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="Actual time" hint={d.targetTimeS != null && d.actualTimeS != null ? `${d.actualTimeS - d.targetTimeS >= 0 ? '+' : ''}${d.actualTimeS - d.targetTimeS}s vs target` : 'm:ss'}>
              <TimeInput seconds={d.actualTimeS} onChange={(actualTimeS) => set({ actualTimeS })} />
            </Field>
            <Field label="Grind setting">
              <input className="input" value={d.grindSetting} onChange={(e) => set({ grindSetting: e.target.value })} placeholder="e.g. 4.2 or 22 clicks" maxLength={40} />
            </Field>
            <Field label="Grinder">
              <select
                className="input"
                value={d.grinderId ?? ''}
                onChange={(e) => {
                  const id = e.target.value ? Number(e.target.value) : null
                  const g = grinders?.find((x) => x.id === id)
                  set({ grinderId: id, burrset: g?.burrsets[0] ?? '' })
                }}
              >
                <option value="">{grinders?.length ? 'No grinder' : 'Add grinders in your profile'}</option>
                {grinders?.map((g) => (
                  <option key={g.id} value={g.id}>
                    {g.nickname ? `${g.nickname} (${g.model})` : g.model}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="Burrs">
              {grinder && grinder.burrsets.length > 1 ? (
                <select className="input" value={d.burrset} onChange={(e) => set({ burrset: e.target.value })}>
                  {grinder.burrsets.map((b) => (
                    <option key={b}>{b}</option>
                  ))}
                </select>
              ) : (
                <input className="input" value={d.burrset} onChange={(e) => set({ burrset: e.target.value })} placeholder={grinder ? 'Stock' : '—'} disabled={!grinder} />
              )}
            </Field>
          </div>
        </section>

        <section className="card space-y-4 p-5">
          <div>
            <h2 className="display text-xl">Flavor</h2>
            <p className="mt-1 text-sm text-ink-2">What did you taste?</p>
          </div>
          <FlavorPicker value={d.descriptors} onChange={(descriptors) => set({ descriptors })} groups={catalog?.flavors ?? []} />
          <Field label="Notes">
            <textarea className="input min-h-24" value={d.notes} onChange={(e) => set({ notes: e.target.value })} placeholder="Tasting notes, pour schedule, what you'd change…" maxLength={4000} />
          </Field>
        </section>
      </div>

      <aside className="space-y-4 lg:sticky lg:top-24 lg:self-start">
        <section className="card space-y-5 p-5">
          <div>
            <span className="label">Your rating</span>
            <StarInput value={d.rating} onChange={(rating) => set({ rating })} />
          </div>
          <Toggle
            checked={d.isPublic}
            onChange={(isPublic) => set({ isPublic, excludeFromGlobal: isPublic ? false : d.excludeFromGlobal })}
            label="Public brew"
            hint="Other members see it once they've rated (or revealed) this coffee. Signed-out visitors see it with spoilers on."
          />
          {!d.isPublic && (
            <Toggle
              checked={d.excludeFromGlobal}
              onChange={(excludeFromGlobal) => set({ excludeFromGlobal })}
              label="Exclude from global ratings"
              hint="Keep this rating out of the community and group averages (e.g. a botched brew)."
            />
          )}
          <div className="border-t border-line pt-4 text-xs text-ink-3">
            {d.ratio != null && d.method && (
              <p>
                {d.method} · {formatRatio({ ...d, ratio: d.ratio })} · {formatTemp(d.tempC, unit)} · {formatTime(d.actualTimeS)}
              </p>
            )}
          </div>
          <ErrorText error={save.error ?? del.error} />
          <button className="btn btn-accent w-full" disabled={save.isPending || !!missing} title={missing ?? undefined}>
            {save.isPending ? 'Saving…' : brewId ? 'Save changes' : 'Log brew'}
          </button>
          {missing && <p className="text-center text-xs text-ink-3">{missing}</p>}
          {brewId && (
            <button
              type="button"
              className="btn btn-danger w-full"
              disabled={del.isPending}
              onClick={() => del.mutate(undefined, { onSuccess: () => navigate({ to: '/brews' }) })}
            >
              <Trash2 size={15} /> Delete brew
            </button>
          )}
        </section>
      </aside>
    </form>
  )
}

export function NewBrewPage() {
  const { coffeeId } = newRoute.useSearch()
  const { isPending } = useQuery(coffeesQuery())
  if (isPending) return <Spinner />
  return (
    <>
      <BackLink />
      <PageHeader eyebrow="New brew" title="Log a brew" />
      <BrewForm initial={emptyDraft(coffeeId)} />
    </>
  )
}

export function EditBrewPage() {
  const { brewId } = editRoute.useParams()
  const { data, isPending, error } = useQuery(brewQuery(brewId))
  if (isPending) return <Spinner />
  if (error) return <ErrorText error={error} />
  if (!data.mine) return <ErrorText error="You can only edit your own brews." />
  return (
    <>
      <BackLink />
      <PageHeader eyebrow={`Day ${data.coffeeDay}${data.coffeeName ? ` · ${data.coffeeName}` : ''}`} title="Edit brew" />
      <BrewForm key={data.id} initial={fromBrew(data)} brewId={data.id} />
    </>
  )
}

function BackLink() {
  return (
    <button onClick={() => history.back()} className="mb-6 inline-flex items-center gap-1 text-sm font-semibold text-ink-2 hover:text-ink">
      <ArrowLeft size={16} /> Back
    </button>
  )
}
