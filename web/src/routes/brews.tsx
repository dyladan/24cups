import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { myBrewsQuery } from '../api'
import { BrewCard } from '../components/BrewCard'
import { Empty, ErrorText, PageHeader, Spinner } from '../components/ui'
import { plural } from '../lib/format'

export function BrewsPage() {
  const { data, isPending, error } = useQuery(myBrewsQuery)
  if (isPending) return <Spinner />
  if (error) return <ErrorText error={error} />
  const rated = data.filter((b) => b.rating != null)
  return (
    <>
      <PageHeader eyebrow={`${plural(data.length, 'brew')} · ${rated.length} rated`} title="My brews">
        <Link to="/brews/new" className="btn btn-accent">
          Log a brew
        </Link>
      </PageHeader>
      {data.length ? (
        <div className="grid gap-3 md:grid-cols-2">
          {data.map((b) => (
            <BrewCard key={b.id} brew={b} showCoffee showUser={false} />
          ))}
        </div>
      ) : (
        <Empty>You haven’t logged any brews yet.</Empty>
      )}
    </>
  )
}
