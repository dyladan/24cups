import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { Eye, Heart, Star } from 'lucide-react'
import { coffeesQuery, meQuery, type Coffee, type Stats } from '../api'
import { Blossom, ErrorText, PageHeader, Spinner, TastingNotes } from '../components/ui'
import { formatRating } from '../lib/format'

export function todayDay(): number | null {
  const d = new Date()
  return d.getMonth() === 11 ? d.getDate() : null
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

function Door({ c, today }: { c: Coffee; today: boolean }) {
  if (!c.opened) {
    return (
      <Link
        to="/coffee/$coffeeId"
        params={{ coffeeId: c.id }}
        className={clsx(
          'group relative flex aspect-[4/5] flex-col justify-between overflow-hidden rounded-2xl p-4 text-door-ink transition hover:-translate-y-0.5 hover:shadow-lg sm:aspect-square xl:aspect-[4/5]',
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
          <span className="relative text-[0.65rem] font-semibold uppercase tracking-[0.2em] opacity-70 transition group-hover:opacity-100">Open door →</span>
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
        'card door-pop group relative flex aspect-[4/5] flex-col justify-between overflow-hidden p-4 transition hover:-translate-y-0.5 hover:border-gold hover:shadow-lg sm:aspect-square xl:aspect-[4/5]',
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
        <TastingNotes notes={info.tastingNotes} className="mt-1.5 line-clamp-2 !text-[0.6rem] !tracking-[0.1em] leading-relaxed" />
      </div>
      {c.stats && <StatRow stats={c.stats} />}
    </Link>
  )
}

export function CalendarPage() {
  const { data, isPending, error } = useQuery(coffeesQuery)
  const { data: me } = useQuery(meQuery)
  const today = todayDay()
  if (isPending) return <Spinner />
  if (error) return <ErrorText error={error} />
  const opened = data.filter((c) => c.opened).length
  return (
    <>
      <PageHeader eyebrow={`Hi ${me?.displayName ?? ''} · ${opened} of ${data.length} opened`} title="Explore the lineup">
        <Link to="/brews/new" className="btn btn-accent">
          Log a brew
        </Link>
      </PageHeader>
      <div className="mb-6 h-1.5 overflow-hidden rounded-full bg-paper-2">
        <div className="h-full rounded-full bg-gold transition-all" style={{ width: `${(opened / Math.max(1, data.length)) * 100}%` }} />
      </div>
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 sm:gap-4 lg:grid-cols-4 xl:grid-cols-6">
        {data.map((c) => (
          <Door key={c.id} c={c} today={today === c.day} />
        ))}
      </div>
    </>
  )
}
