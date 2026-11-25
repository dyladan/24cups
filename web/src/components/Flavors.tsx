import { useState, type KeyboardEvent } from 'react'
import clsx from 'clsx'
import { ChevronDown, Plus, X } from 'lucide-react'
import type { Flavors } from '../api'
import { plural } from '../lib/format'

export const MAX_FLAVORS = 10
const MAX_LEN = 30
/** Matches flavorMinTasters on the server. */
const MIN_TASTERS = 3
const TOP_N = 6

/** Flavors are stored lowercase; show them with a leading capital. */
export function flavorLabel(f: string) {
  return f.charAt(0).toUpperCase() + f.slice(1)
}

const normalize = (s: string) => s.trim().toLowerCase().replace(/\s+/g, ' ')

/** Read-only tags on a brew card. */
export function FlavorChips({ flavors, className }: { flavors: string[]; className?: string }) {
  if (!flavors.length) return null
  return (
    <ul className={clsx('flex flex-wrap gap-1.5', className)} aria-label="Flavors">
      {flavors.map((f) => (
        <li key={f} className="chip">
          {flavorLabel(f)}
        </li>
      ))}
    </ul>
  )
}

/** Tag picker for the brew form: suggestion groups plus free text. */
export function FlavorPicker({
  value,
  onChange,
  groups,
}: {
  value: string[]
  onChange: (v: string[]) => void
  groups: { group: string; items: string[] }[]
}) {
  const [text, setText] = useState('')
  const [open, setOpen] = useState<string | null>(groups[0]?.group ?? null)
  const full = value.length >= MAX_FLAVORS
  const all = groups.flatMap((g) => g.items)

  function add(raw: string) {
    const f = normalize(raw).slice(0, MAX_LEN)
    setText('')
    if (!f || value.includes(f) || full) return
    onChange([...value, f])
  }
  const remove = (f: string) => onChange(value.filter((x) => x !== f))
  const toggle = (f: string) => (value.includes(f) ? remove(f) : add(f))

  function onKey(e: KeyboardEvent<HTMLInputElement>) {
    if (e.key === 'Enter' || e.key === ',') {
      // Don't submit the brew form; add the tag instead.
      e.preventDefault()
      add(text)
    } else if (e.key === 'Backspace' && !text && value.length) {
      remove(value[value.length - 1])
    }
  }

  return (
    <div className="space-y-3">
      {value.length > 0 && (
        <ul className="flex flex-wrap gap-1.5" aria-label="Your flavors">
          {value.map((f) => (
            <li key={f} className="chip !border-ink !py-1 !text-ink">
              {flavorLabel(f)}
              <button type="button" onClick={() => remove(f)} aria-label={`Remove ${f}`} className="text-ink-3 hover:text-ink">
                <X size={12} />
              </button>
            </li>
          ))}
        </ul>
      )}
      <div className="flex gap-2">
        <input
          className="input"
          list="flavor-suggestions"
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={onKey}
          // Don't lose a typed flavor if the user moves on without pressing Enter.
          onBlur={() => text.trim() && add(text)}
          placeholder={full ? `That's ${MAX_FLAVORS}, the most per brew` : 'Add a flavor… (type or pick below)'}
          maxLength={MAX_LEN}
          disabled={full}
          autoComplete="off"
          aria-label="Add a flavor"
        />
        <datalist id="flavor-suggestions">
          {all
            .filter((f) => !value.includes(f))
            .map((f) => (
              <option key={f} value={flavorLabel(f)} />
            ))}
        </datalist>
        <button type="button" className="btn btn-ghost" onClick={() => add(text)} disabled={!text.trim() || full}>
          <Plus size={15} /> Add
        </button>
      </div>
      <div className="divide-y divide-line rounded-xl border border-line">
        {groups.map((g) => {
          const isOpen = open === g.group
          const picked = g.items.filter((f) => value.includes(f)).length
          return (
            <div key={g.group}>
              <button
                type="button"
                className="flex w-full items-center justify-between px-3.5 py-2.5 text-left text-sm font-semibold"
                onClick={() => setOpen(isOpen ? null : g.group)}
                aria-expanded={isOpen}
              >
                <span>
                  {g.group}
                  {picked > 0 && <span className="ml-2 text-xs font-normal text-ink-3">{picked} picked</span>}
                </span>
                <ChevronDown size={16} className={clsx('text-ink-3 transition', isOpen && 'rotate-180')} />
              </button>
              {isOpen && (
                <div className="flex flex-wrap gap-1.5 px-3.5 pb-3">
                  {g.items.map((f) => {
                    const on = value.includes(f)
                    return (
                      <button
                        key={f}
                        type="button"
                        aria-pressed={on}
                        disabled={!on && full}
                        onClick={() => toggle(f)}
                        className={clsx(
                          'chip !py-1 transition disabled:opacity-40',
                          on ? '!border-heading bg-heading !text-paper' : 'hover:border-ink hover:text-ink',
                        )}
                      >
                        {flavorLabel(f)}
                      </button>
                    )
                  })}
                </div>
              )}
            </div>
          )
        })}
      </div>
      <p className="text-xs text-ink-3">
        Up to {MAX_FLAVORS} per brew. Anything you type works; new words show on the coffee page once a second person uses them.
      </p>
    </div>
  )
}

/** "What people taste" on the coffee page (only once revealed). */
export function FlavorPanel({ flavors }: { flavors: Flavors }) {
  const [all, setAll] = useState(false)
  const { tasters, top } = flavors
  const list = top ?? []
  const shown = all ? list : list.slice(0, TOP_N)
  return (
    <section className="card p-5">
      <div className="mb-3 flex items-baseline justify-between gap-2">
        <p className="eyebrow">What people taste</p>
        {tasters > 0 && <span className="text-xs text-ink-3 tabular-nums">{plural(tasters, 'taster')}</span>}
      </div>
      {top == null || list.length === 0 ? (
        <p className="text-sm text-ink-2">
          {tasters === 0
            ? 'Nobody has tagged flavors yet. Add some when you log a brew.'
            : `Shown once ${MIN_TASTERS} people have tagged flavors (${tasters} so far).`}
        </p>
      ) : (
        <>
          <ul className="space-y-2">
            {shown.map((f) => {
              const pct = Math.round((f.count / tasters) * 100)
              return (
                <li
                  key={f.name}
                  className="grid grid-cols-[minmax(0,7.5rem)_1fr_2.25rem_0.75rem] items-center gap-2 text-sm"
                  title={`${plural(f.count, 'taster')} of ${tasters}`}
                >
                  <span className="truncate font-medium">{flavorLabel(f.name)}</span>
                  <span className="h-2 overflow-hidden rounded-full bg-line">
                    <span className="block h-full rounded-full bg-gold" style={{ width: `${Math.max(4, pct)}%` }} />
                  </span>
                  <span className="text-right text-xs text-ink-3 tabular-nums">{pct}%</span>
                  <span className="text-accent" aria-label={f.mine ? 'You tasted it' : undefined}>
                    {f.mine ? '●' : ''}
                  </span>
                </li>
              )
            })}
          </ul>
          <div className="mt-3 flex items-center justify-between gap-2 text-xs text-ink-3">
            {list.some((f) => f.mine) ? (
              <span>
                <span className="text-accent">●</span> you tasted it
              </span>
            ) : (
              <span />
            )}
            {list.length > TOP_N && (
              <button type="button" className="font-semibold hover:text-ink" onClick={() => setAll(!all)}>
                {all ? 'Show fewer' : `Show all ${list.length}`}
              </button>
            )}
          </div>
        </>
      )}
    </section>
  )
}
