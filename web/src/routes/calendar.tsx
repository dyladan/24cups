import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { Eye, EyeOff, Heart, Star } from 'lucide-react'
import { coffeesQuery, meQuery, type Coffee, type Stats } from '../api'
import { useCoffeeView } from '../lib/anon'
import { Blossom, ErrorText, PageHeader, Spinner, TastingNotes } from '../components/ui'
import { formatRating } from '../lib/format'

export function todayDay(): number | null {
  const d = new Date()
  return d.getMonth() === 11 ? d.getDate() : null
}

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

function Door({ c, today, anon }: { c: Coffee; today: boolean; anon: boolean }) {
  if (!c.opened) {
    return (
      <Link
        to="/coffee/$coffeeId"
        params={{ coffeeId: c.id }}
        className={clsx(
          'group relative flex min-h-52 flex-col justify-between overflow-hidden rounded-2xl p-4 text-door-ink transition hover:-translate-y-0.5 hover:shadow-lg md:aspect-square',
          today ? 'bg-[#141110] ring-2 ring-door-ink/70' : 'bg-door',
        )}
      >
        {/* foil frame */}
        <span className="pointer-events-none absolute inset-2 rounded-xl border border-door-ink/25" />
        <div className="relative flex items-start justify-between">
          <span className="text-[0.62rem] font-semibold uppercase tracking-[0.3em] opacity-80">{today ? 'Today' : 'Day'}</span>
          <Blossom size={18} className="opacity-60 transition group-hover:rotate-45 group-hover:opacity-100" />
        </div>
        <span className="display relative !text-door-ink text-7xl sm:text-8xl">{String(c.day).padStart(2, '0')}</span>
        {c.stats ? (
          <StatRow stats={c.stats} className="relative !text-door-ink [&_*]:!text-door-ink" />
        ) : (
          <span className="relative text-[0.65rem] font-semibold uppercase tracking-[0.2em] opacity-70 transition group-hover:opacity-100">
            {anon ? (today ? 'Hidden · spoilers to peek' : `Opens Dec ${c.day}`) : 'Open door →'}
          </span>
        )}
      </Link>
    )
  }
  const info = c.info!
  return (
    <Link
      to="/coffee/$coffeeId"
      params={{ coffeeId: c.id }}
      className={clsx(
        'card door-pop group relative flex min-h-52 flex-col justify-between p-4 transition hover:-translate-y-0.5 hover:border-gold hover:shadow-lg md:aspect-square',
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
      {c.revealed ? c.stats && <StatRow stats={c.stats} /> : <BlindRow stats={c.stats} anon={anon} />}
    </Link>
  )
}

export function CalendarPage() {
  const { view, ready, anon } = useCoffeeView()
  const { data, isPending, error } = useQuery({ ...coffeesQuery(view), enabled: ready })
  const { data: me } = useQuery(meQuery)
  const today = todayDay()
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
          A community log for this year’s coffee advent calendar. Past days show everything: tasting notes, ratings and brews.
          Today’s door stays sealed so members who haven’t opened it yet aren’t spoiled; switch on{' '}
          <span className="font-semibold">Spoilers</span> at the top to peek.
        </p>
      )}
      <div className="mb-6 h-1.5 overflow-hidden rounded-full bg-paper-2">
        <div className="h-full rounded-full bg-gold transition-all" style={{ width: `${(opened / Math.max(1, data.length)) * 100}%` }} />
      </div>
      <div className="grid auto-rows-fr grid-cols-2 gap-3 sm:grid-cols-3 sm:gap-4 md:auto-rows-auto lg:grid-cols-4">
        {data.map((c) => (
          <Door key={c.id} c={c} today={today === c.day} anon={anon} />
        ))}
      </div>
    </>
  )
}
