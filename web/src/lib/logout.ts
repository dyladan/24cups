import { useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { api } from '../api'

/** Drops cached data for the account and goes to the sign-in page. */
export function useSignedOut() {
  const qc = useQueryClient()
  const navigate = useNavigate()
  return () => {
    qc.removeQueries({ predicate: (q) => q.queryKey[0] !== 'me' })
    qc.setQueryData(['me'], null)
    navigate({ to: '/login' })
  }
}

/** Signs out, drops cached data for the account, and goes to the sign-in page. */
export function useLogout() {
  const signedOut = useSignedOut()
  return async () => {
    await api('/auth/logout', { method: 'POST' })
    signedOut()
  }
}
