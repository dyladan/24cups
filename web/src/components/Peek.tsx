import { useState, type MouseEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { Eye, EyeOff } from 'lucide-react'
import { peekQuery } from '../api'

/**
 * Peek state for a sealed door: shows the coffee's name and origin without
 * opening it. Nothing is saved; it resets when the page is left or reloaded.
 */
export function usePeek(coffeeId: number) {
  const [on, setOn] = useState(false)
  const { data, isFetching, error } = useQuery({ ...peekQuery(coffeeId), enabled: on })
  return { on, peek: on ? data : undefined, loading: on && isFetching && !data, error, toggle: () => setOn((v) => !v) }
}

export function PeekButton({ on, loading, onToggle, className }: { on: boolean; loading?: boolean; onToggle: () => void; className?: string }) {
  return (
    <button
      type="button"
      aria-pressed={on}
      onClick={(e: MouseEvent) => {
        e.preventDefault()
        e.stopPropagation()
        onToggle()
      }}
      className={clsx(
        'inline-flex items-center gap-1 rounded-full border px-2.5 py-1 text-[0.65rem] font-semibold uppercase tracking-[0.15em] transition',
        className,
      )}
    >
      {on ? <EyeOff size={12} /> : <Eye size={12} />}
      {loading ? '…' : on ? 'Hide' : 'Peek'}
    </button>
  )
}
