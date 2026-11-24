import { useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { api } from '../api'

/** Signs out, drops cached data for the account, and goes to the sign-in page. */
export function useLogout() {
  const qc = useQueryClient()
  const navigate = useNavigate()
  return async () => {
    await api('/auth/logout', { method: 'POST' })
    qc.removeQueries({ predicate: (q) => q.queryKey[0] !== 'me' })
    qc.setQueryData(['me'], null)
    navigate({ to: '/login' })
  }
}
