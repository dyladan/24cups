import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { EyeOff, Heart } from 'lucide-react'
import { coffeesQuery, type Coffee } from '../api'
import { Empty, ErrorText, PageHeader, Segmented, Spinner, Stars } from '../components/ui'
import { formatRating } from '../lib/format'

type SortKey = 'global' | 'mine' | 'favorites' | 'day'

// Coffees still blind for this user (opened, not yet rated) sort last.
const blindLast = (a: Coffee, b: Coffee) => Number(!a.revealed) - Number(!b.revealed)

const sorters: Record<SortKey, (a: Coffee, b: Coffee) => number> = {
  global: (a, b) => blindLast(a, b) || (b.stats?.avgRating ?? -1) - (a.stats?.avgRating ?? -1) || a.day - b.day,
  mine: (a, b) => (b.myAvg ?? -1) - (a.myAvg ?? -1) || a.day - b.day,
  favorites: (a, b) => blindLast(a, b) || (b.stats?.favorites ?? 0) - (a.stats?.favorites ?? 0) || a.day - b.day,
  day: (a, b) => a.day - b.day,
}

export function RankingsPage() {
  const { data, isPending, error } = useQuery(coffeesQuery())
  const [sort, setSort] = useState<SortKey>('global')
  if (isPending) return <Spinner />
  if (error) return <ErrorText error={error} />
  const list = data.filter((c) => c.opened).sort(sorters[sort])
  const hidden = data.length - list.length
  return (
    <>
      <PageHeader eyebrow="Opened coffees" title="Rankings" />
      <div className="mb-5 overflow-x-auto">
        <Segmented<SortKey>
          label="Sort by"
          value={sort}
          onChange={setSort}
          options={[
            { value: 'global', label: 'Community' },
            { value: 'mine', label: 'Mine' },
            { value: 'favorites', label: 'Favorites' },
            { value: 'day', label: 'Day' },
          ]}
        />
      </div>
      {list.length === 0 ? (
        <Empty>Open some doors to start ranking.</Empty>
      ) : (
        <ol className="card divide-y divide-line overflow-hidden">
          {list.map((c, i) => (
            <li key={c.id}>
              <Link to="/coffee/$coffeeId" params={{ coffeeId: c.id }} className="flex items-center gap-4 px-4 py-3.5 transition hover:bg-paper-2 sm:px-5">
                <span className="display w-8 text-2xl text-ink-3 tabular-nums">{sort === 'day' ? c.day : i + 1}</span>
                <div className="min-w-0 flex-1">
                  <p className="truncate font-semibold">
                    {c.info?.name}
                    {c.favorited && <Heart size={13} className="ml-1.5 inline text-accent" fill="currentColor" />}
                  </p>
                  <p className="truncate text-xs text-ink-3">
                    Day {c.day}
                    {c.info?.origin && ` · ${c.info.origin}`}
                    {c.info?.process && ` · ${c.info.process}`}
                  </p>
                </div>
                <div className="hidden text-right sm:block">
                  <p className="eyebrow">You</p>
                  <p className="text-sm font-semibold tabular-nums">{formatRating(c.myAvg)}</p>
                </div>
                <div className="hidden w-16 text-right sm:block">
                  <p className="eyebrow">Favs</p>
                  <p className="text-sm font-semibold tabular-nums">{c.revealed ? (c.stats?.favorites ?? 0) : '—'}</p>
                </div>
                <div className="w-24 text-right">
                  {sort !== 'mine' && !c.revealed ? (
                    <p className="inline-flex items-center gap-1 text-xs text-ink-3">
                      <EyeOff size={12} /> Rate to reveal
                    </p>
                  ) : (
                    <>
                      <p className="text-lg font-bold tabular-nums">{formatRating(sort === 'mine' ? c.myAvg : c.stats?.avgRating)}</p>
                      <Stars value={sort === 'mine' ? c.myAvg : c.stats?.avgRating} size={12} />
                    </>
                  )}
                </div>
              </Link>
            </li>
          ))}
        </ol>
      )}
      {hidden > 0 && <p className="mt-4 text-center text-xs text-ink-3">{hidden} unopened coffees are hidden.</p>}
    </>
  )
}
