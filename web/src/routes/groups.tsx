import { useEffect, useRef, useState, type FormEvent } from 'react'
import { Link, getRouteApi, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { ArrowLeft, Check, Copy, EyeOff, Globe, Lock, Pencil, RefreshCw, UserMinus, Users } from 'lucide-react'
import { api, discoverGroupsQuery, groupQuery, meQuery, myGroupsQuery, useApiMutation, type Group, type GroupDetail } from '../api'
import { BrewCard } from '../components/BrewCard'
import { Empty, ErrorText, Field, PageHeader, Spinner, Stars, Toggle } from '../components/ui'
import { formatRating, plural } from '../lib/format'

const groupRoute = getRouteApi('/app/groups/$groupId')
const joinRoute = getRouteApi('/app/join/$code')

function GroupRow({ g }: { g: Group }) {
  return (
    <Link to="/groups/$groupId" params={{ groupId: g.id }} className="card flex items-center gap-4 p-4 transition hover:border-ink">
      <div className="flex size-11 shrink-0 items-center justify-center rounded-xl bg-ink text-paper">
        <span className="display text-xl">{g.name.slice(0, 1)}</span>
      </div>
      <div className="min-w-0 flex-1">
        <p className="truncate font-semibold">{g.name}</p>
        <p className="truncate text-xs text-ink-3">
          {plural(g.memberCount, 'member')} · {g.isPublic ? 'Public' : 'Private'}
          {g.description && ` · ${g.description}`}
        </p>
      </div>
      {g.isPublic ? <Globe size={16} className="text-ink-3" /> : <Lock size={16} className="text-ink-3" />}
    </Link>
  )
}

export function GroupsPage() {
  const navigate = useNavigate()
  const { data: mine, isPending, error } = useQuery(myGroupsQuery)
  const [q, setQ] = useState('')
  const { data: discover } = useQuery(discoverGroupsQuery(q))

  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [isPublic, setIsPublic] = useState(false)
  const create = useApiMutation((body: object) => api<Group>('/groups', { method: 'POST', json: body }), [['groups']])

  const [code, setCode] = useState('')
  const join = useApiMutation((c: string) => api<Group>('/groups/join', { method: 'POST', json: { code: c } }), [['groups'], ['coffees']])

  if (isPending) return <Spinner />
  if (error) return <ErrorText error={error} />

  const notMine = discover?.filter((g) => !g.role) ?? []

  return (
    <>
      <PageHeader eyebrow="Taste together" title="Groups" />
      <div className="grid gap-8 lg:grid-cols-[1fr_360px]">
        <div className="space-y-8">
          <section>
            <h2 className="display mb-3 text-2xl">Your groups</h2>
            {mine.length ? (
              <div className="grid gap-3 sm:grid-cols-2">
                {mine.map((g) => (
                  <GroupRow key={g.id} g={g} />
                ))}
              </div>
            ) : (
              <Empty>You’re not in any groups yet. Create one or join with an invite code.</Empty>
            )}
          </section>
          <section>
            <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
              <h2 className="display text-2xl">Public groups</h2>
              <input className="input !w-full sm:!w-64" placeholder="Search…" value={q} onChange={(e) => setQ(e.target.value)} />
            </div>
            {notMine.length ? (
              <div className="grid gap-3 sm:grid-cols-2">
                {notMine.map((g) => (
                  <GroupRow key={g.id} g={g} />
                ))}
              </div>
            ) : (
              <Empty>{q ? 'No public groups match.' : 'No other public groups yet.'}</Empty>
            )}
          </section>
        </div>

        <aside className="space-y-4">
          <form
            className="card space-y-4 p-5"
            onSubmit={(e: FormEvent) => {
              e.preventDefault()
              create.mutate({ name, description, isPublic }, { onSuccess: (g) => navigate({ to: '/groups/$groupId', params: { groupId: g.id } }) })
            }}
          >
            <h2 className="display text-xl">Create a group</h2>
            <Field label="Name">
              <input className="input" value={name} onChange={(e) => setName(e.target.value)} required maxLength={60} />
            </Field>
            <Field label="Description">
              <textarea className="input min-h-16" value={description} onChange={(e) => setDescription(e.target.value)} maxLength={500} />
            </Field>
            <Toggle checked={isPublic} onChange={setIsPublic} label="Public" hint="Anyone can find and join. Private groups need an invite code." />
            <ErrorText error={create.error} />
            <button className="btn btn-primary w-full" disabled={create.isPending}>
              Create group
            </button>
          </form>
          <form
            className="card space-y-3 p-5"
            onSubmit={(e: FormEvent) => {
              e.preventDefault()
              join.mutate(code, { onSuccess: (g) => navigate({ to: '/groups/$groupId', params: { groupId: g.id } }) })
            }}
          >
            <h2 className="display text-xl">Join with a code</h2>
            <input className="input font-mono" value={code} onChange={(e) => setCode(e.target.value)} placeholder="Invite code or link" required />
            <ErrorText error={join.error} />
            <button className="btn btn-ghost w-full" disabled={join.isPending}>
              Join
            </button>
          </form>
        </aside>
      </div>
    </>
  )
}

function InviteBox({ g }: { g: Group }) {
  const [copied, setCopied] = useState(false)
  const regen = useApiMutation(() => api(`/groups/${g.id}/invite-code`, { method: 'POST' }), [['groups']])
  const link = `${location.origin}/join/${g.inviteCode}`
  async function copy() {
    try {
      if (navigator.share && matchMedia('(pointer: coarse)').matches) {
        await navigator.share({ title: `Join ${g.name} on Onyx Advent`, url: link })
      } else {
        await navigator.clipboard.writeText(link)
        setCopied(true)
        setTimeout(() => setCopied(false), 1500)
      }
    } catch {
      /* user cancelled */
    }
  }
  return (
    <section className="card space-y-3 p-5">
      <p className="eyebrow">Invite link</p>
      <div className="flex gap-2">
        <input className="input font-mono text-xs" readOnly value={link} onFocus={(e) => e.target.select()} />
        <button className="btn btn-primary !px-3" onClick={copy} aria-label="Copy invite link">
          {copied ? <Check size={16} /> : <Copy size={16} />}
        </button>
      </div>
      <p className="text-xs text-ink-3">
        Code <span className="font-mono">{g.inviteCode}</span>
        {g.role === 'owner' && (
          <button className="ml-2 inline-flex items-center gap-1 font-semibold text-ink-2 hover:text-ink" onClick={() => regen.mutate()} disabled={regen.isPending}>
            <RefreshCw size={11} /> New code
          </button>
        )}
      </p>
    </section>
  )
}

function GroupNameForm({ g, current, profileName, onDone }: { g: GroupDetail; current: string; profileName: string; onDone: () => void }) {
  const [name, setName] = useState(current)
  const save = useApiMutation((displayName: string) => api(`/groups/${g.id}/name`, { method: 'PUT', json: { displayName } }), [['groups']])
  return (
    <form
      className="mt-1.5 space-y-2"
      onSubmit={(e: FormEvent) => {
        e.preventDefault()
        save.mutate(name, { onSuccess: onDone })
      }}
    >
      <input className="input !py-1.5 text-sm" value={name} onChange={(e) => setName(e.target.value)} maxLength={50} placeholder={profileName} autoFocus aria-label="Your name in this group" />
      <p className="text-xs text-ink-3">Only shown in {g.name}. Leave blank to use your profile name.</p>
      <ErrorText error={save.error} />
      <div className="flex gap-2">
        <button className="btn btn-primary !px-3 !py-1 text-sm" disabled={save.isPending || name.trim() === current}>
          Save
        </button>
        <button type="button" className="btn btn-ghost !px-3 !py-1 text-sm" onClick={onDone}>
          Cancel
        </button>
      </div>
    </form>
  )
}

export function GroupPage() {
  const { groupId } = groupRoute.useParams()
  const navigate = useNavigate()
  const { data: me } = useQuery(meQuery)
  const { data: g, isPending, error } = useQuery(groupQuery(groupId))
  const join = useApiMutation(() => api(`/groups/${groupId}/join`, { method: 'POST' }), [['groups']])
  const leave = useApiMutation(() => api(`/groups/${groupId}/leave`, { method: 'POST' }), [['groups']])
  const del = useApiMutation(() => api(`/groups/${groupId}`, { method: 'DELETE' }), [['groups']])
  const kick = useApiMutation((uid: number) => api(`/groups/${groupId}/members/${uid}`, { method: 'DELETE' }), [['groups']])
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [editingName, setEditingName] = useState(false)
  const [sort, setSort] = useState<'rating' | 'day'>('rating')

  if (isPending) return <Spinner />
  if (error) return <ErrorText error={error} />

  const isOwner = g.role === 'owner'
  // Opened but not yet rated by the viewer: scores hidden, listed last.
  const blind = (c: GroupDetail['coffees'][number]) => c.name != null && !c.revealed
  const ranked = [...g.coffees]
    .filter((c) => c.stats)
    .sort((a, b) =>
      sort === 'day'
        ? a.day - b.day
        : Number(blind(a)) - Number(blind(b)) || (b.stats!.avgRating ?? -1) - (a.stats!.avgRating ?? -1) || a.day - b.day,
    )

  return (
    <>
      <Link to="/groups" className="mb-6 inline-flex items-center gap-1 text-sm font-semibold text-ink-2 hover:text-ink">
        <ArrowLeft size={16} /> Groups
      </Link>
      <PageHeader eyebrow={`${g.isPublic ? 'Public' : 'Private'} group · ${plural(g.memberCount, 'member')}`} title={g.name}>
        {!g.role && (
          <button className="btn btn-accent" onClick={() => join.mutate()} disabled={join.isPending}>
            Join group
          </button>
        )}
        {g.role === 'member' && (
          <button className="btn btn-ghost" onClick={() => leave.mutate(undefined, { onSuccess: () => navigate({ to: '/groups' }) })} disabled={leave.isPending}>
            Leave
          </button>
        )}
        {isOwner &&
          (confirmDelete ? (
            <>
              <button className="btn btn-danger" onClick={() => del.mutate(undefined, { onSuccess: () => navigate({ to: '/groups' }) })} disabled={del.isPending}>
                Really delete?
              </button>
              <button className="btn btn-ghost" onClick={() => setConfirmDelete(false)}>
                Cancel
              </button>
            </>
          ) : (
            <button className="btn btn-ghost" onClick={() => setConfirmDelete(true)}>
              Delete group
            </button>
          ))}
      </PageHeader>
      {g.description && <p className="-mt-3 mb-6 max-w-prose text-ink-2">{g.description}</p>}
      <ErrorText error={join.error ?? leave.error ?? del.error ?? kick.error} />

      <div className="grid gap-8 lg:grid-cols-[1fr_340px]">
        <div className="min-w-0 space-y-8">
          <section>
            <div className="mb-3 flex items-center justify-between gap-3">
              <h2 className="display text-2xl">Group leaderboard</h2>
              <div className="seg">
                <button aria-pressed={sort === 'rating'} onClick={() => setSort('rating')}>
                  Rating
                </button>
                <button aria-pressed={sort === 'day'} onClick={() => setSort('day')}>
                  Day
                </button>
              </div>
            </div>
            {ranked.length ? (
              <ol className="card divide-y divide-line overflow-hidden">
                {ranked.map((c, i) => (
                  <li key={c.coffeeId}>
                    <Link to="/coffee/$coffeeId" params={{ coffeeId: c.coffeeId }} className="flex items-center gap-3 px-4 py-3 hover:bg-paper-2">
                      <span className="display w-7 text-xl text-ink-3">{sort === 'day' ? c.day : i + 1}</span>
                      <div className="min-w-0 flex-1">
                        <p className={clsx('truncate font-semibold', !c.name && 'text-ink-3')}>{c.name ?? `Day ${c.day} (unopened)`}</p>
                        <p className="text-xs text-ink-3">
                          Day {c.day} · {c.stats!.opens} opened{!blind(c) && ` · ${c.stats!.favorites} ♥`}
                        </p>
                      </div>
                      {blind(c) ? (
                        <p className="inline-flex shrink-0 items-center gap-1 text-xs text-ink-3">
                          <EyeOff size={12} /> Rate to reveal
                        </p>
                      ) : (
                        <div className="text-right">
                          <p className="font-bold tabular-nums">{formatRating(c.stats!.avgRating)}</p>
                          <Stars value={c.stats!.avgRating} size={11} />
                        </div>
                      )}
                    </Link>
                  </li>
                ))}
              </ol>
            ) : (
              <Empty>Open some doors to see how this group rates them.</Empty>
            )}
          </section>
          <section>
            <h2 className="display mb-3 text-2xl">Recent brews</h2>
            {g.brews.length ? (
              <div className="space-y-3">
                {g.brews.map((b) => (
                  <BrewCard key={b.id} brew={b} showCoffee />
                ))}
              </div>
            ) : (
              <Empty>No shared brews from this group for the coffees you’ve opened.</Empty>
            )}
          </section>
        </div>
        <aside className="space-y-4">
          {g.role && g.inviteCode && <InviteBox g={g} />}
          <section className="card p-5">
            <p className="eyebrow mb-3 flex items-center gap-1.5">
              <Users size={13} /> Members
            </p>
            <ul className="space-y-2.5">
              {g.members.map((m) => (
                <li key={m.userId} className={clsx('flex gap-3', m.userId === me?.id && editingName ? 'items-start' : 'items-center')}>
                  <div className="flex size-8 shrink-0 items-center justify-center rounded-full bg-paper-2 text-xs font-bold uppercase">
                    {m.displayName.slice(0, 2)}
                  </div>
                  <div className="min-w-0 flex-1">
                    <p className="flex items-center gap-1.5 text-sm font-medium">
                      <span className="truncate">{m.displayName}</span>
                      {m.userId === me?.id && <span className="shrink-0 text-ink-3">(you)</span>}
                      {m.userId === me?.id && !editingName && (
                        <button className="shrink-0 text-ink-3 hover:text-ink" onClick={() => setEditingName(true)} aria-label="Change your name in this group" title="Change your name in this group">
                          <Pencil size={12} />
                        </button>
                      )}
                    </p>
                    {m.groupName && <p className="truncate text-xs text-ink-3">aka {m.profileName}</p>}
                    <p className="text-xs text-ink-3">
                      {m.role === 'owner' ? 'Owner · ' : ''}
                      {m.opens} opened · {plural(m.brews, 'shared brew')}
                    </p>
                    {m.userId === me?.id && editingName && (
                      <GroupNameForm g={g} current={m.groupName} profileName={m.profileName} onDone={() => setEditingName(false)} />
                    )}
                  </div>
                  {isOwner && m.userId !== me?.id && (
                    <button className="text-ink-3 hover:text-red-600" onClick={() => kick.mutate(m.userId)} aria-label={`Remove ${m.displayName}`} title="Remove from group">
                      <UserMinus size={16} />
                    </button>
                  )}
                </li>
              ))}
            </ul>
          </section>
        </aside>
      </div>
    </>
  )
}

export function JoinPage() {
  const { code } = joinRoute.useParams()
  const navigate = useNavigate()
  const join = useApiMutation((c: string) => api<Group>('/groups/join', { method: 'POST', json: { code: c } }), [['groups']])
  const started = useRef(false)
  useEffect(() => {
    if (started.current) return
    started.current = true
    join.mutate(code, { onSuccess: (g) => navigate({ to: '/groups/$groupId', params: { groupId: g.id }, replace: true }) })
  }, [code, join, navigate])
  return join.error ? (
    <div className="mx-auto max-w-md space-y-4 py-10 text-center">
      <ErrorText error={join.error} />
      <Link to="/groups" className="btn btn-ghost">
        Back to groups
      </Link>
    </div>
  ) : (
    <Spinner />
  )
}
