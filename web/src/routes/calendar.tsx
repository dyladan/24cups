import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { Eye, EyeOff, Heart, Star } from 'lucide-react'
import { api, coffeeKeys, coffeeQuery, coffeesQuery, meQuery, useApiMutation, type Coffee, type Stats } from '../api'
import { useCoffeeView, useToday, useTodayPeek, type CoffeeView } from '../lib/anon'
import { PeekButton, usePeek } from '../components/Peek'
import { Blossom, ErrorText, PageHeader, Spinner, TastingNotes } from '../components/ui'
import { formatRating } from '../lib/format'
import { ONYX_URL } from '../lib/links'

/** Shown instead of scores while a coffee is still blind for this user. */
export function BlindRow({ stats, className, anon }: { stats: Stats | null; className?: string; anon?: boolean }) {
  return (
    <div className={clsx('flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-ink-3', className)}>
      <span className="inline-flex items-center gap-1" title={anon ? 'Turn on spoilers to see ratings' : 'Scores hidden until you rate it'}>
        <EyeOff size={13} /> {anon ? 'Ratings hidden' : 'Rate to reveal'}
      </span>
      {stats && (
        <span className="inline-flex items-center gap-1 tabular-nums" title="People who opened this door">
          <Eye size={13} /> {stats.opens}
        </span>
      )}
    </div>
  )
}

export function StatRow({ stats, className }: { stats: Stats; className?: string }) {
  return (
    <div className={clsx('flex flex-wrap items-center gap-x-3 gap-y-1 text-xs tabular-nums text-ink-2', className)}>
      <span className="inline-flex items-center gap-1" title={`Average of ${stats.ratingCount} rater(s)`}>
        <Star size={13} className="text-gold" fill="currentColor" strokeWidth={0} />
        <span className="font-semibold text-ink">{formatRating(stats.avgRating)}</span>
        <span className="text-ink-3">({stats.ratingCount})</span>
      </span>
      <span className="inline-flex items-center gap-1" title="People who opened this door">
        <Eye size={13} /> {stats.opens}
      </span>
      <span className="inline-flex items-center gap-1" title="Favorites">
        <Heart size={13} /> {stats.favorites}
      </span>
    </div>
  )
}

/**
 * Signed out, today's Peek button opens just that door with spoilers (its
 * name already shows), without turning on the site-wide Spoilers switch.
 */
function AnonTodayDoor({ c, view }: { c: Coffee; view: CoffeeView | null }) {
  const [spoil, setSpoil] = useTodayPeek(c.id)
  const { data, isFetching } = useQuery({ ...coffeeQuery(c.id, view && { ...view, spoilers: true }), enabled: spoil && !!view })
  if (spoil && data?.opened) {
    return (
      <div className="relative h-full">
        <Door c={data} today anon view={view} actionSpace />
        <PeekButton
          on
          onToggle={() => setSpoil(false)}
          className="absolute right-4 bottom-4 z-10 border-line bg-card text-ink-2 hover:text-ink"
        />
      </div>
    )
  }
  return <SealedDoor c={c} today anon peekLoading={spoil && isFetching} onPeek={() => setSpoil(true)} />
}

function SealedDoor({
  c,
  today,
  future,
  anon,
  onPeek,
  peekLoading,
}: {
  c: Coffee
  today: boolean
  future?: boolean
  anon: boolean
  /** Replaces the name-only peek (today's door, signed out). */
  onPeek?: () => void
  peekLoading?: boolean
}) {
  const { on, peek: peeked, loading, toggle } = usePeek(c.id)
  // Today's name and origin come with the calendar; other doors need a peek.
  const peek = c.preview ?? peeked
  // Signed in, today's door opens right here; other doors go to their page.
  const opensHere = today && !anon
  const open = useApiMutation(() => api(`/coffees/${c.id}/open`, { method: 'POST' }), coffeeKeys)
  const hint = anon
    ? today
      ? 'Hidden today'
      : `Opens Dec ${c.day}`
    : opensHere
      ? open.isPending
        ? 'Opening…'
        : open.error
          ? 'Couldn’t open, try again'
          : 'Open door →'
      : future
        ? null
        : 'Open door →'
  return (
    <div
      className={clsx(
        'group relative flex h-full min-h-52 flex-col justify-between overflow-hidden rounded-2xl p-4 text-door-ink transition hover:-translate-y-0.5 hover:shadow-lg md:aspect-square',
        'bg-door',
        today && 'ring-2 ring-gold',
      )}
    >
      {/* The whole door is the link (or open button); content sits above it but lets clicks through. */}
      {opensHere ? (
        <button
          type="button"
          className="absolute inset-0 cursor-pointer"
          onClick={() => open.mutate()}
          disabled={open.isPending}
          aria-label={`Open door ${c.day}`}
        />
      ) : (
        <Link to="/coffee/$coffeeId" params={{ coffeeId: c.id }} className="absolute inset-0" aria-label={`Day ${c.day}`} />
      )}
      {/* foil frame */}
      <span className="pointer-events-none absolute inset-2 rounded-xl border border-door-ink/25" />
      <div className="pointer-events-none relative flex items-start justify-between">
        <span className="text-[0.62rem] font-semibold uppercase tracking-[0.3em] opacity-80">
          {today ? 'Today' : peek ? 'Day' : null}
          {peek && ` ${c.day}`}
        </span>
        <Blossom size={18} className="opacity-60 transition group-hover:rotate-45 group-hover:opacity-100" />
      </div>
      {peek ? (
        <div className="pointer-events-none relative min-w-0">
          {peek.origin && <p className="mb-1 truncate text-[0.62rem] font-semibold uppercase tracking-[0.24em] opacity-80">{peek.origin}</p>}
          <p className="display line-clamp-3 !text-door-ink text-2xl sm:text-3xl">{peek.name}</p>
        </div>
      ) : (
        <span className="display pointer-events-none relative !text-door-ink text-7xl sm:text-8xl">{String(c.day).padStart(2, '0')}</span>
      )}
      <div className="pointer-events-none relative flex items-end justify-between gap-2">
        {c.stats ? (
          <StatRow stats={c.stats} className="!text-door-ink [&_*]:!text-door-ink" />
        ) : (
          <span className="text-[0.65rem] font-semibold uppercase tracking-[0.2em] opacity-70 transition group-hover:opacity-100">{hint}</span>
        )}
        {(onPeek || !c.preview) && (
          <PeekButton
            on={onPeek ? false : on}
            loading={onPeek ? peekLoading : loading}
            onToggle={onPeek ?? toggle}
            className="pointer-events-auto relative z-10 shrink-0 border-door-ink/40 text-door-ink hover:bg-door-ink/15"
          />
        )}
      </div>
    </div>
  )
}

function Door({
  c,
  today,
  future,
  anon,
  view,
  actionSpace,
}: {
  c: Coffee
  today: boolean
  future?: boolean
  anon: boolean
  view: CoffeeView | null
  /** Keep the bottom-right corner free for a button laid over the card. */
  actionSpace?: boolean
}) {
  if (!c.opened) {
    if (anon && today) return <AnonTodayDoor c={c} view={view} />
    return <SealedDoor c={c} today={today} future={future} anon={anon} />
  }
  const info = c.info!
  return (
    <Link
      to="/coffee/$coffeeId"
      params={{ coffeeId: c.id }}
      className={clsx(
        'card door-pop group relative flex h-full min-h-52 flex-col justify-between p-4 transition hover:-translate-y-0.5 hover:border-gold hover:shadow-lg md:aspect-square',
        today && 'ring-2 ring-gold',
      )}
    >
      <div className="flex items-start justify-between gap-2">
        <span className="display !text-gold text-3xl">{String(c.day).padStart(2, '0')}</span>
        <span className="flex items-center gap-1.5">
          {c.myAvg != null && (
            <span className="chip border-gold/60 !px-2 tabular-nums" title="Your average">
              {formatRating(c.myAvg)}
              <Star size={10} className="text-gold" fill="currentColor" strokeWidth={0} />
            </span>
          )}
          {c.favorited && <Heart size={16} className="text-accent" fill="currentColor" />}
        </span>
      </div>
      <div className="min-w-0">
        <p className="eyebrow mb-1 truncate">{info.origin}</p>
        <h3 className="display line-clamp-2 text-xl sm:text-2xl">{info.name}</h3>
        {(c.revealed || anon) && <TastingNotes notes={info.tastingNotes} className="mt-1.5 line-clamp-6 !text-[0.6rem] !tracking-[0.1em] leading-relaxed" />}
      </div>
      {c.revealed ? (
        c.stats && <StatRow stats={c.stats} className={clsx(actionSpace && 'min-h-6 pr-18')} />
      ) : (
        <BlindRow stats={c.stats} anon={anon} className={clsx(actionSpace && 'min-h-6 pr-18')} />
      )}
    </Link>
  )
}

export function CalendarPage() {
  const { view, ready, anon } = useCoffeeView()
  const { data, isPending, error } = useQuery({ ...coffeesQuery(view), enabled: ready })
  const { data: me } = useQuery(meQuery)
  const { day: today, isFuture } = useToday()
  if (isPending) return <Spinner />
  if (error) return <ErrorText error={error} />
  const opened = data.filter((c) => c.opened).length
  const eyebrow = anon
    ? today != null && today <= data.length
      ? `Day ${today} of ${data.length}`
      : opened === 0
        ? 'Doors open December 1'
        : `All ${data.length} doors open`
    : `Hi ${me?.displayName ?? ''} · ${opened} of ${data.length} opened`
  return (
    <>
      <PageHeader eyebrow={eyebrow} title="Explore the lineup">
        {anon ? (
          <Link to="/register" className="btn btn-accent">
            Join to log &amp; rate
          </Link>
        ) : (
          <Link to="/brews/new" className="btn btn-accent">
            Log a brew
          </Link>
        )}
      </PageHeader>
      {anon && (
        <p className="-mt-3 mb-6 max-w-2xl text-sm text-ink-2">
          Community ratings for the{' '}
          <a href={ONYX_URL} target="_blank" rel="noopener noreferrer" className="font-semibold text-ink underline decoration-gold underline-offset-2 hover:decoration-2">
            2026 Onyx Coffee Lab advent calendar
          </a>
          . 24 coffees, one behind each door. Explore the list or{' '}
          <Link to="/login" className="font-semibold text-ink underline decoration-gold underline-offset-2 hover:decoration-2">
            sign in
          </Link>{' '}
          to rate brews.
        </p>
      )}
      <div className="mb-6 h-1.5 overflow-hidden rounded-full bg-paper-2">
        <div className="h-full rounded-full bg-gold transition-all" style={{ width: `${(opened / Math.max(1, data.length)) * 100}%` }} />
      </div>
      <div className="grid auto-rows-fr grid-cols-2 gap-3 sm:grid-cols-3 sm:gap-4 md:auto-rows-auto lg:grid-cols-4">
        {data.map((c) => (
          <Door key={c.id} c={c} today={today === c.day} future={isFuture(c.day)} anon={anon} view={view} />
        ))}
      </div>
    </>
  )
}
