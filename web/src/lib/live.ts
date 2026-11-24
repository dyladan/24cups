import { useEffect } from 'react'
import { useQueryClient, type QueryKey } from '@tanstack/react-query'
import { clientId } from '../api'

/**
 * Subscribes to server-sent "invalidate" hints and refetches affected
 * queries. Hints carry no data; refetches go through the normal API.
 */
export function useLiveUpdates(enabled: boolean) {
  const qc = useQueryClient()
  useEffect(() => {
    if (!enabled) return
    const es = new EventSource(`/api/events?client=${encodeURIComponent(clientId)}`)
    const pending = new Map<string, QueryKey>()
    let timer: ReturnType<typeof setTimeout> | undefined
    let connectedBefore = false

    const flush = () => {
      timer = undefined
      for (const queryKey of pending.values()) qc.invalidateQueries({ queryKey })
      pending.clear()
    }

    es.addEventListener('invalidate', (e) => {
      try {
        const { keys } = JSON.parse((e as MessageEvent).data) as { keys: QueryKey[] }
        for (const k of keys) pending.set(JSON.stringify(k), k)
      } catch {
        return
      }
      // Coalesce bursts (e.g. several people brewing at once) into one refetch.
      timer ??= setTimeout(flush, 250)
    })

    es.addEventListener('open', () => {
      // After a reconnect we may have missed hints; refresh everything once.
      if (connectedBefore) qc.invalidateQueries()
      connectedBefore = true
    })

    return () => {
      es.close()
      if (timer) clearTimeout(timer)
    }
  }, [enabled, qc])
}
