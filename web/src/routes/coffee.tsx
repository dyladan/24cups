import { Link, getRouteApi } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { ArrowLeft, ChevronLeft, ChevronRight, DoorOpen, Heart, Plus, Undo2 } from 'lucide-react'
import { api, coffeeBrewsQuery, coffeeKeys, coffeeQuery, coffeesQuery, useApiMutation, type CoffeeDetail } from '../api'
import { BrewCard } from '../components/BrewCard'
import { Empty, ErrorText, Spinner, Stars } from '../components/ui'
import { formatRating, plural } from '../lib/format'
import { StatRow } from './calendar'

const route = getRouteApi('/app/coffee/$coffeeId')

function Distribution({ dist }: { dist: number[] }) {
  const max = Math.max(1, ...dist)
  return (
    <div>
      <div className="flex h-24 items-end gap-1" aria-label="Rating distribution">
        {dist.map((n, i) => (
          <div key={i} className="flex flex-1 flex-col items-center justify-end" title={`${(i + 1) / 2}★: ${n}`}>
            <div className={clsx('w-full rounded-t-sm', n ? 'bg-gold' : 'bg-line')} style={{ height: `${Math.max(4, (n / max) * 100)}%` }} />
          </div>
        ))}
      </div>
      <div className="mt-1 flex justify-between text-[0.6rem] text-ink-3">
        <span>½★</span>
        <span>5★</span>
      </div>
    </div>
  )
}

function StatsPanel({ c }: { c: CoffeeDetail }) {
  const s = c.stats!
  return (
    <section className="card p-5">
      <p className="eyebrow mb-3">Community</p>
      <div className="flex items-end gap-3">
        {s.avgRating == null ? (
          <span className="display text-3xl text-ink-3">No ratings yet</span>
        ) : (
          <span className="display text-6xl tabular-nums">{formatRating(s.avgRating)}</span>
        )}
        <div className="pb-1.5">
          <Stars value={s.avgRating} size={18} />
          <p className="text-xs text-ink-3">
            {plural(s.ratingCount, 'rater')} · {plural(s.brewCount, 'brew')}
          </p>
        </div>
      </div>
      <StatRow stats={s} className="mt-3" />
      {c.myAvg != null && (
        <p className="mt-3 text-sm">
          Your average: <span className="font-semibold">{formatRating(c.myAvg)}</span>
        </p>
      )}
      {c.distribution && s.brewCount > 0 && (
        <div className="mt-5">
          <Distribution dist={c.distribution} />
        </div>
      )}
      {!!c.groupStats?.length && (
        <div className="mt-5 border-t border-line pt-4">
          <p className="eyebrow mb-2">Your groups</p>
          <ul className="space-y-2">
            {c.groupStats.map((g) => (
              <li key={g.groupId} className="flex items-center justify-between gap-2 text-sm">
                <Link to="/groups/$groupId" params={{ groupId: g.groupId }} className="truncate font-medium hover:underline">
                  {g.groupName}
                </Link>
                <span className="flex shrink-0 items-center gap-1.5 tabular-nums">
                  <Stars value={g.avgRating} size={12} />
                  <span className="w-8 text-right font-semibold">{formatRating(g.avgRating)}</span>
                  <span className="text-xs text-ink-3">({g.ratingCount})</span>
                </span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </section>
  )
}

export function CoffeePage() {
  const { coffeeId } = route.useParams()
  const { data: c, isPending, error } = useQuery(coffeeQuery(coffeeId))
  const { data: all } = useQuery(coffeesQuery)
  const { data: brews } = useQuery({ ...coffeeBrewsQuery(coffeeId), enabled: !!c?.opened })

  const open = useApiMutation(() => api(`/coffees/${coffeeId}/open`, { method: 'POST' }), coffeeKeys)
  const unopen = useApiMutation(() => api(`/coffees/${coffeeId}/open`, { method: 'DELETE' }), coffeeKeys)
  const fav = useApiMutation(
    (on: boolean) => api(`/coffees/${coffeeId}/favorite`, { method: on ? 'PUT' : 'DELETE' }),
    coffeeKeys,
  )

  if (isPending) return <Spinner />
  if (error) return <ErrorText error={error} />

  const idx = all?.findIndex((x) => x.id === c.id) ?? -1
  const prev = idx > 0 ? all![idx - 1] : null
  const next = all && idx >= 0 && idx < all.length - 1 ? all[idx + 1] : null
  const mine = brews?.filter((b) => b.mine) ?? []
  const others = brews?.filter((b) => !b.mine) ?? []

  const nav = (
    <div className="mb-6 flex items-center justify-between">
      <Link to="/" className="inline-flex items-center gap-1 text-sm font-semibold text-ink-2 hover:text-ink">
        <ArrowLeft size={16} /> Calendar
      </Link>
      <div className="flex gap-1">
        {prev && (
          <Link to="/coffee/$coffeeId" params={{ coffeeId: prev.id }} className="btn btn-ghost !px-2.5 !py-1.5" aria-label={`Day ${prev.day}`}>
            <ChevronLeft size={16} /> {prev.day}
          </Link>
        )}
        {next && (
          <Link to="/coffee/$coffeeId" params={{ coffeeId: next.id }} className="btn btn-ghost !px-2.5 !py-1.5" aria-label={`Day ${next.day}`}>
            {next.day} <ChevronRight size={16} />
          </Link>
        )}
      </div>
    </div>
  )

  if (!c.opened) {
    return (
      <>
        {nav}
        <div className="mx-auto max-w-md text-center">
          <div className="relative mx-auto flex aspect-[4/5] w-56 flex-col items-center justify-center rounded-3xl bg-ink text-paper shadow-xl">
            <span className="text-xs font-semibold uppercase tracking-[0.2em] opacity-60">Day</span>
            <span className="display text-[7rem]">{String(c.day).padStart(2, '0')}</span>
            <span className="absolute inset-y-4 right-3 border-r border-dashed border-current opacity-20" />
          </div>
          <h1 className="display mt-8 text-3xl">Still sealed</h1>
          <p className="mt-2 text-ink-2">Opening reveals the coffee, its community scores and everyone’s brews. You don’t have to log a brew to open it.</p>
          <button className="btn btn-accent mt-6" onClick={() => open.mutate()} disabled={open.isPending}>
            <DoorOpen size={18} /> Open door {c.day}
          </button>
          <ErrorText error={open.error} />
          {c.stats && (
            <div className="card mt-8 p-4 text-left">
              <p className="eyebrow mb-2">Community (spoilers on)</p>
              <StatRow stats={c.stats} />
            </div>
          )}
        </div>
      </>
    )
  }

  const info = c.info!
  const facts: [string, string][] = (
    [
      ['Origin', info.origin],
      ['Region', info.region],
      ['Producer', info.producer],
      ['Process', info.process],
      ['Variety', info.variety],
      ['Elevation', info.elevation],
      ['Roast', info.roastLevel],
    ] as [string, string][]
  ).filter(([, v]) => v)

  return (
    <>
      {nav}
      <div className="grid gap-6 lg:grid-cols-[1fr_340px] lg:gap-10">
        <div className="min-w-0">
          <p className="eyebrow mb-2">Day {c.day}</p>
          <h1 className="display text-5xl sm:text-6xl">{info.name}</h1>
          {info.tastingNotes && <p className="mt-3 text-lg text-ink-2">{info.tastingNotes}</p>}

          <div className="mt-5 flex flex-wrap gap-2">
            <Link to="/brews/new" search={{ coffeeId: c.id }} className="btn btn-accent">
              <Plus size={18} /> Log a brew
            </Link>
            <button
              className={clsx('btn', c.favorited ? 'btn-primary' : 'btn-ghost')}
              onClick={() => fav.mutate(!c.favorited)}
              disabled={fav.isPending}
              aria-pressed={c.favorited}
            >
              <Heart size={16} fill={c.favorited ? 'currentColor' : 'none'} className={c.favorited ? 'text-accent' : ''} />
              {c.favorited ? 'Favorited' : 'Favorite'}
            </button>
          </div>

          {facts.length > 0 && (
            <dl className="mt-8 grid grid-cols-2 gap-x-6 gap-y-4 border-y border-line py-5 sm:grid-cols-3">
              {facts.map(([k, v]) => (
                <div key={k}>
                  <dt className="eyebrow">{k}</dt>
                  <dd className="mt-0.5 font-medium">{v}</dd>
                </div>
              ))}
            </dl>
          )}
          {info.description && <p className="mt-6 max-w-prose leading-relaxed text-ink-2">{info.description}</p>}

          <section className="mt-10">
            <h2 className="display mb-4 text-2xl">Your brews</h2>
            {mine.length ? (
              <div className="space-y-3">
                {mine.map((b) => (
                  <BrewCard key={b.id} brew={b} showUser={false} />
                ))}
              </div>
            ) : (
              <Empty>
                No brews yet.{' '}
                <Link to="/brews/new" search={{ coffeeId: c.id }} className="font-semibold text-ink underline underline-offset-4">
                  Log your first one
                </Link>
                .
              </Empty>
            )}
          </section>

          <section className="mt-10">
            <h2 className="display mb-4 text-2xl">Community brews</h2>
            {others.length ? (
              <div className="space-y-3">
                {others.map((b) => (
                  <BrewCard key={b.id} brew={b} />
                ))}
              </div>
            ) : (
              <Empty>Nobody has shared a brew of this one yet.</Empty>
            )}
          </section>
        </div>

        <aside className="space-y-4 lg:sticky lg:top-24 lg:self-start">
          <StatsPanel c={c} />
          {c.myBrewCount === 0 && (
            <button className="inline-flex items-center gap-1.5 text-xs text-ink-3 hover:text-ink" onClick={() => unopen.mutate()} disabled={unopen.isPending}>
              <Undo2 size={13} /> Opened by mistake? Close this door
            </button>
          )}
          <ErrorText error={unopen.error ?? fav.error} />
        </aside>
      </div>
    </>
  )
}
