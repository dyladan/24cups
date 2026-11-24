import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { DoorClosed, Eye, Heart, Star } from 'lucide-react'
import { coffeesQuery, meQuery, type Coffee, type Stats } from '../api'
import { ErrorText, PageHeader, Spinner } from '../components/ui'
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
          'group relative flex aspect-[4/5] flex-col justify-between overflow-hidden rounded-2xl border p-4 transition hover:-translate-y-0.5 hover:shadow-lg sm:aspect-square',
          today ? 'border-accent bg-accent text-accent-ink' : 'border-ink bg-ink text-paper',
        )}
      >
        <div className="flex items-start justify-between">
          <span className="text-[0.65rem] font-semibold uppercase tracking-[0.18em] opacity-60">{today ? 'Today' : 'Day'}</span>
          <DoorClosed size={18} className="opacity-50 transition group-hover:opacity-100" />
        </div>
        <span className="display text-7xl sm:text-8xl">{String(c.day).padStart(2, '0')}</span>
        {c.stats ? (
          <StatRow stats={c.stats} className="!text-current opacity-80 [&_*]:!text-current" />
        ) : (
          <span className="text-xs font-medium opacity-60 transition group-hover:opacity-100">Open door →</span>
        )}
        {/* little "perforation" detail along the door edge */}
        <span className="pointer-events-none absolute inset-y-3 right-2 border-r border-dashed border-current opacity-20" />
      </Link>
    )
  }
  const info = c.info!
  return (
    <Link
      to="/coffee/$coffeeId"
      params={{ coffeeId: c.id }}
      className={clsx(
        'card door-pop group flex aspect-[4/5] flex-col justify-between p-4 transition hover:-translate-y-0.5 hover:border-ink hover:shadow-lg sm:aspect-square',
        today && 'ring-2 ring-accent',
      )}
    >
      <div className="flex items-start justify-between gap-2">
        <span className="display text-3xl text-ink-3">{String(c.day).padStart(2, '0')}</span>
        {c.favorited && <Heart size={18} className="text-accent" fill="currentColor" />}
      </div>
      <div className="min-w-0">
        <h3 className="display line-clamp-2 text-xl sm:text-2xl">{info.name}</h3>
        <p className="mt-1 truncate text-xs text-ink-2">{[info.origin, info.process].filter(Boolean).join(' · ') || ' '}</p>
        <div className="mt-2 flex flex-wrap gap-1">
          {info.roastLevel && <span className="chip">{info.roastLevel}</span>}
          {c.myAvg != null && (
            <span className="chip border-gold/60">
              You {formatRating(c.myAvg)}
              <Star size={10} className="text-gold" fill="currentColor" strokeWidth={0} />
            </span>
          )}
        </div>
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
      <PageHeader eyebrow={`Hi ${me?.displayName ?? ''} · ${opened} of ${data.length} opened`} title="The Calendar">
        <Link to="/brews/new" className="btn btn-accent">
          Log a brew
        </Link>
      </PageHeader>
      <div className="mb-6 h-1.5 overflow-hidden rounded-full bg-paper-2">
        <div className="h-full rounded-full bg-accent transition-all" style={{ width: `${(opened / Math.max(1, data.length)) * 100}%` }} />
      </div>
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 sm:gap-4 lg:grid-cols-4 xl:grid-cols-6">
        {data.map((c) => (
          <Door key={c.id} c={c} today={today === c.day} />
        ))}
      </div>
    </>
  )
}
