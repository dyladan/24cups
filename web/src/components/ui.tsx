import { useState, type ReactNode } from 'react'
import clsx from 'clsx'
import { Star, StarHalf, X } from 'lucide-react'
import { cToF, fToC, formatTime, parseTime, round } from '../lib/format'

// ---------- layout bits ----------

export function Field({ label, hint, children, className }: { label: string; hint?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <label className={clsx('block', className)}>
      <span className="label">{label}</span>
      {children}
      {hint && <span className="mt-1 block text-xs text-ink-3">{hint}</span>}
    </label>
  )
}

export function ErrorText({ error }: { error: unknown }) {
  if (!error) return null
  const msg = error instanceof Error ? error.message : String(error)
  return <p className="rounded-xl bg-red-600/10 px-3 py-2 text-sm text-red-700 dark:text-red-400">{msg}</p>
}

export function PageHeader({ eyebrow, title, children }: { eyebrow?: string; title: ReactNode; children?: ReactNode }) {
  return (
    <div className="mb-6 flex flex-wrap items-end justify-between gap-4 sm:mb-8">
      <div>
        {eyebrow && <p className="eyebrow mb-2">{eyebrow}</p>}
        <h1 className="display text-4xl sm:text-5xl">{title}</h1>
      </div>
      {children && <div className="flex flex-wrap gap-2">{children}</div>}
    </div>
  )
}

export function Empty({ children }: { children: ReactNode }) {
  return <div className="card border-dashed px-6 py-10 text-center text-sm text-ink-2">{children}</div>
}

export function Spinner() {
  return (
    <div className="flex justify-center py-16" role="status" aria-label="Loading">
      <div className="size-6 animate-spin rounded-full border-2 border-line border-t-ink" />
    </div>
  )
}

export function Segmented<T extends string>({
  value,
  options,
  onChange,
  label,
}: {
  value: T
  options: { value: T; label: ReactNode }[]
  onChange: (v: T) => void
  label: string
}) {
  return (
    <div className="seg" role="group" aria-label={label}>
      {options.map((o) => (
        <button type="button" key={o.value} aria-pressed={o.value === value} onClick={() => onChange(o.value)}>
          {o.label}
        </button>
      ))}
    </div>
  )
}

export function Toggle({ checked, onChange, label, hint, disabled }: { checked: boolean; onChange: (v: boolean) => void; label: string; hint?: ReactNode; disabled?: boolean }) {
  return (
    <label className={clsx('flex cursor-pointer items-start gap-3', disabled && 'cursor-not-allowed opacity-50')}>
      <button
        type="button"
        role="switch"
        aria-checked={checked}
        disabled={disabled}
        onClick={() => onChange(!checked)}
        className={clsx(
          'relative mt-0.5 inline-flex h-6 w-10 shrink-0 rounded-full transition',
          checked ? 'bg-heading' : 'bg-line',
        )}
      >
        <span className={clsx('absolute top-0.5 size-5 rounded-full bg-paper shadow transition-all', checked ? 'left-[1.125rem]' : 'left-0.5')} />
      </button>
      <span>
        <span className="block text-sm font-medium">{label}</span>
        {hint && <span className="block text-xs text-ink-3">{hint}</span>}
      </span>
    </label>
  )
}

// ---------- stars ----------

/** Read-only star display; value on a 0–5 scale. */
export function Stars({ value, size = 16, className }: { value: number | null | undefined; size?: number; className?: string }) {
  const v = value ?? 0
  return (
    <span className={clsx('inline-flex items-center text-gold', className)} aria-label={value == null ? 'No rating' : `${round(v, 2)} out of 5`}>
      {[1, 2, 3, 4, 5].map((i) => {
        const fill = Math.max(0, Math.min(1, v - (i - 1)))
        return (
          <span key={i} className="relative inline-block" style={{ width: size, height: size }}>
            <Star size={size} className="absolute inset-0 text-line" fill="currentColor" strokeWidth={0} />
            <span className="absolute inset-0 overflow-hidden" style={{ width: `${fill * 100}%` }}>
              <Star size={size} fill="currentColor" strokeWidth={0} />
            </span>
          </span>
        )
      })}
    </span>
  )
}

/** Half-star rating input; value is 1–10 (half-stars) or null. */
export function StarInput({ value, onChange }: { value: number | null; onChange: (v: number | null) => void }) {
  const [hover, setHover] = useState<number | null>(null)
  const shown = hover ?? value ?? 0
  return (
    <div className="flex items-center gap-3">
      <div className="flex" onMouseLeave={() => setHover(null)} role="radiogroup" aria-label="Rating">
        {[1, 2, 3, 4, 5].map((i) => {
          const full = shown >= i * 2
          const half = !full && shown >= i * 2 - 1
          return (
            <span key={i} className="relative size-10 text-gold sm:size-9">
              {full ? (
                <Star className="size-full" fill="currentColor" strokeWidth={0} />
              ) : half ? (
                <span className="relative block size-full">
                  <Star className="absolute inset-0 size-full text-line" fill="currentColor" strokeWidth={0} />
                  <StarHalf className="absolute inset-0 size-full" fill="currentColor" strokeWidth={0} />
                </span>
              ) : (
                <Star className="size-full text-line" fill="currentColor" strokeWidth={0} />
              )}
              {[i * 2 - 1, i * 2].map((v, j) => (
                <button
                  key={v}
                  type="button"
                  role="radio"
                  aria-checked={value === v}
                  aria-label={`${v / 2} stars`}
                  className={clsx('absolute inset-y-0 w-1/2', j === 0 ? 'left-0' : 'right-0')}
                  onMouseEnter={() => setHover(v)}
                  onClick={() => onChange(v)}
                />
              ))}
            </span>
          )
        })}
      </div>
      <span className="w-10 text-sm font-semibold tabular-nums">{shown ? (shown / 2).toFixed(1) : '—'}</span>
      {value != null && (
        <button type="button" className="text-ink-3 hover:text-ink" onClick={() => onChange(null)} aria-label="Clear rating">
          <X size={16} />
        </button>
      )}
    </div>
  )
}

// ---------- numeric inputs ----------

function numOrNull(s: string): number | null {
  if (s.trim() === '') return null
  const n = Number(s)
  return Number.isFinite(n) ? n : null
}

type RatioMode = 'ratio' | 'inout' | 'gl'

/**
 * Brew ratio entry in three styles. The canonical value is ratio (water per
 * gram of coffee, the X in 1:X) plus an optional dose.
 */
export function RatioInput({
  doseG,
  ratio,
  method,
  onChange,
}: {
  doseG: number | null
  ratio: number | null
  method: string
  onChange: (v: { doseG: number | null; ratio: number | null }) => void
}) {
  const espresso = method.toLowerCase().includes('espresso')
  const [mode, setMode] = useState<RatioMode>(espresso ? 'inout' : 'ratio')
  const [dose, setDose] = useState(doseG != null ? String(doseG) : '')
  const [second, setSecond] = useState(() => secondFor(mode, doseG, ratio))

  function secondFor(m: RatioMode, d: number | null, r: number | null): string {
    if (r == null) return ''
    if (m === 'ratio') return String(round(r, 2))
    if (m === 'gl') return String(round(1000 / r, 1))
    return d != null ? String(round(d * r, 1)) : ''
  }

  function emit(m: RatioMode, doseStr: string, secondStr: string) {
    const d = numOrNull(doseStr)
    const s = numOrNull(secondStr)
    let r: number | null = null
    if (s != null && s > 0) {
      if (m === 'ratio') r = s
      else if (m === 'gl') r = 1000 / s
      else if (d != null && d > 0) r = s / d
    }
    onChange({ doseG: d, ratio: r })
  }

  const outUnit = espresso ? 'g out' : 'mL out'
  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="label mb-0">Brew ratio</span>
        <Segmented<RatioMode>
          label="Ratio format"
          value={mode}
          onChange={(m) => {
            setMode(m)
            setSecond(secondFor(m, numOrNull(dose), ratio))
          }}
          options={[
            { value: 'ratio', label: '1:X' },
            { value: 'inout', label: 'g in / out' },
            { value: 'gl', label: 'g/L' },
          ]}
        />
      </div>
      <div className="grid grid-cols-2 gap-2">
        <div className="relative">
          <input
            className="input pr-14"
            inputMode="decimal"
            placeholder={espresso ? '18' : '15'}
            value={dose}
            aria-label="Coffee dose in grams"
            onChange={(e) => {
              setDose(e.target.value)
              emit(mode, e.target.value, second)
            }}
          />
          <span className="pointer-events-none absolute inset-y-0 right-3 flex items-center text-xs text-ink-3">g in</span>
        </div>
        <div className="relative">
          {mode === 'ratio' && <span className="pointer-events-none absolute inset-y-0 left-3.5 flex items-center text-sm text-ink-3">1 :</span>}
          <input
            className={clsx('input pr-16', mode === 'ratio' && 'pl-9')}
            inputMode="decimal"
            placeholder={mode === 'ratio' ? (espresso ? '2' : '16') : mode === 'gl' ? '62.5' : espresso ? '36' : '250'}
            value={second}
            aria-label={mode === 'ratio' ? 'Ratio' : mode === 'gl' ? 'Grams per litre' : 'Output'}
            onChange={(e) => {
              setSecond(e.target.value)
              emit(mode, dose, e.target.value)
            }}
          />
          <span className="pointer-events-none absolute inset-y-0 right-3 flex items-center text-xs text-ink-3">
            {mode === 'ratio' ? 'ratio' : mode === 'gl' ? 'g/L' : outUnit}
          </span>
        </div>
      </div>
      <p className="text-xs text-ink-3">
        {ratio != null ? (
          <>
            1:{round(ratio, 2)} · {round(1000 / ratio, 1)} g/L
            {doseG != null && ` · ${round(doseG, 1)}g → ${round(doseG * ratio, 0)}${espresso ? 'g' : 'mL'}`}
          </>
        ) : mode === 'inout' ? (
          'Enter both the dose and the output.'
        ) : (
          'Dose is optional for 1:X and g/L.'
        )}
      </p>
    </div>
  )
}

export function TempInput({ tempC, unit, onChange }: { tempC: number | null; unit: 'C' | 'F'; onChange: (c: number | null) => void }) {
  const toDisplay = (c: number | null) => (c == null ? '' : String(unit === 'F' ? Math.round(cToF(c)) : round(c, 1)))
  const [text, setText] = useState(toDisplay(tempC))
  return (
    <div className="relative">
      <input
        className="input pr-10"
        inputMode="decimal"
        placeholder={unit === 'F' ? '205' : '96'}
        value={text}
        onChange={(e) => {
          setText(e.target.value)
          const n = numOrNull(e.target.value)
          onChange(n == null ? null : unit === 'F' ? round(fToC(n), 2) : n)
        }}
      />
      <span className="pointer-events-none absolute inset-y-0 right-3 flex items-center text-xs text-ink-3">°{unit}</span>
    </div>
  )
}

export function TimeInput({ seconds, onChange, placeholder = '3:00' }: { seconds: number | null; onChange: (s: number | null) => void; placeholder?: string }) {
  const [text, setText] = useState(seconds == null ? '' : formatTime(seconds))
  const parsed = parseTime(text)
  const invalid = Number.isNaN(parsed)
  return (
    <input
      className={clsx('input tabular-nums', invalid && 'border-red-500')}
      inputMode="text"
      placeholder={placeholder}
      value={text}
      aria-invalid={invalid}
      onChange={(e) => {
        setText(e.target.value)
        const p = parseTime(e.target.value)
        onChange(p == null || Number.isNaN(p) ? null : p)
      }}
      onBlur={() => {
        if (!invalid && parsed != null) setText(formatTime(parsed))
      }}
    />
  )
}

/** Free-text input with suggestions (native datalist, works on mobile). */
export function SuggestInput({ id, value, onChange, options, placeholder, required }: { id: string; value: string; onChange: (v: string) => void; options: string[]; placeholder?: string; required?: boolean }) {
  return (
    <>
      <input className="input" list={id} value={value} placeholder={placeholder} required={required} onChange={(e) => onChange(e.target.value)} autoComplete="off" />
      <datalist id={id}>
        {options.map((o) => (
          <option key={o} value={o} />
        ))}
      </datalist>
    </>
  )
}

// ---------- brand bits ----------

/** Tasting notes like the lineup card: gold tracked caps separated by bars. */
export function TastingNotes({ notes, className }: { notes: string; className?: string }) {
  const list = notes.split(',').map((n) => n.trim()).filter(Boolean)
  if (!list.length) return null
  return (
    <p className={clsx('notes', className)}>
      {list.map((n, i) => (
        <span key={n}>
          {i > 0 && <span className="opacity-50"> | </span>}
          {n}
        </span>
      ))}
    </p>
  )
}

/** Five-petal coffee blossom used as a small decorative mark. */
export function Blossom({ size = 16, className }: { size?: number; className?: string }) {
  return (
    <svg viewBox="0 0 24 24" width={size} height={size} className={className} aria-hidden="true" fill="currentColor">
      {[0, 72, 144, 216, 288].map((r) => (
        <path key={r} d="M12 12 C10.2 8.5 10.6 4.2 12 1.5 C13.4 4.2 13.8 8.5 12 12Z" transform={`rotate(${r} 12 12)`} />
      ))}
      <circle cx="12" cy="12" r="1.6" fill="var(--paper)" />
    </svg>
  )
}
