import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { EyeOff, Lock, Pencil, Users } from 'lucide-react'
import { meQuery, type Brew } from '../api'
import { formatDate, formatRatio, formatTemp, formatTime } from '../lib/format'
import { Stars } from './ui'
import { FlavorChips } from './Flavors'

export function BrewCard({ brew, showCoffee = false, showUser = true }: { brew: Brew; showCoffee?: boolean; showUser?: boolean }) {
  const { data: me } = useQuery(meQuery)
  const unit = me?.tempUnit ?? 'C'
  const timeDiff = brew.actualTimeS != null && brew.targetTimeS != null ? brew.actualTimeS - brew.targetTimeS : null
  return (
    <article className="card p-4">
      <header className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          {showCoffee && (
            <Link to="/coffee/$coffeeId" params={{ coffeeId: brew.coffeeId }} className="eyebrow mb-1 block hover:text-ink">
              Day {brew.coffeeDay}
              {brew.coffeeName && ` · ${brew.coffeeName}`}
            </Link>
          )}
          <p className="font-semibold">
            {brew.method}
            {showUser && <span className="font-normal text-ink-2"> by {brew.mine ? 'you' : brew.displayName}</span>}
          </p>
          <p className="text-xs text-ink-3">{formatDate(brew.brewedAt)}</p>
        </div>
        <div className="flex shrink-0 flex-col items-end gap-1">
          {brew.rating != null ? <Stars value={brew.rating / 4} size={15} /> : <span className="text-xs text-ink-3">Unrated</span>}
          <div className="flex items-center gap-2 text-ink-3">
            {brew.visibility === 'private' && (
              <span title="Private brew">
                <Lock size={13} />
              </span>
            )}
            {brew.visibility === 'groups' && (
              <span title="Visible to your groups">
                <Users size={13} />
              </span>
            )}
            {brew.excludeFromGlobal && (
              <span title="Excluded from ratings">
                <EyeOff size={13} />
              </span>
            )}
            {brew.mine && (
              <Link to="/brews/$brewId/edit" params={{ brewId: brew.id }} className="hover:text-ink" aria-label="Edit brew">
                <Pencil size={14} />
              </Link>
            )}
          </div>
        </div>
      </header>
      <dl className="mt-3 grid grid-cols-2 gap-x-4 gap-y-2 text-sm sm:grid-cols-4">
        <div>
          <dt className="eyebrow">Ratio</dt>
          <dd className="tabular-nums">{formatRatio(brew)}</dd>
        </div>
        <div>
          <dt className="eyebrow">Temp</dt>
          <dd className="tabular-nums">{formatTemp(brew.tempC, unit)}</dd>
        </div>
        <div>
          <dt className="eyebrow">Time</dt>
          <dd className="tabular-nums">
            {formatTime(brew.actualTimeS)}
            {brew.targetTimeS != null && <span className="text-ink-3"> / {formatTime(brew.targetTimeS)}</span>}
            {timeDiff != null && timeDiff !== 0 && (
              <span className="ml-1 text-xs text-ink-3">
                ({timeDiff > 0 ? '+' : '−'}
                {Math.abs(timeDiff)}s)
              </span>
            )}
          </dd>
        </div>
        <div>
          <dt className="eyebrow">Grind</dt>
          <dd className="truncate" title={[brew.grinderLabel, brew.burrset, brew.grindSetting].filter(Boolean).join(' · ')}>
            {brew.grindSetting || '—'}
            {brew.grinderLabel && <span className="text-ink-3"> on {brew.grinderLabel}</span>}
          </dd>
        </div>
      </dl>
      {brew.burrset && <p className="mt-2 text-xs text-ink-3">Burrs: {brew.burrset}</p>}
      <FlavorChips flavors={brew.descriptors ?? []} className="mt-3" />
      {brew.notes && <p className="mt-3 border-t border-line pt-3 text-sm whitespace-pre-wrap text-ink-2">{brew.notes}</p>}
    </article>
  )
}
