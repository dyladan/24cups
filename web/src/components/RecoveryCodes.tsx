import { useState } from 'react'
import { Check, Copy, Download } from 'lucide-react'

/** One-time recovery codes, shown once. Saving them is the user's job. */
export function RecoveryCodes({ codes, username, onDone, doneLabel = 'Continue' }: { codes: string[]; username: string; onDone: () => void; doneLabel?: string }) {
  const [saved, setSaved] = useState(false)
  const [copied, setCopied] = useState(false)
  const text = `24 Cups recovery codes for @${username}\nEach code signs you in once.\n\n${codes.join('\n')}\n`
  return (
    <div className="space-y-4">
      <p className="text-sm text-ink-2">
        If you lose access to your passkeys, sign in with your username and one of these codes. Each one works once. Keep them
        somewhere safe, like your password manager. <strong>This is the only time they’re shown.</strong>
      </p>
      <ol className="grid grid-cols-2 gap-2 rounded-xl border border-line p-3 font-mono text-sm">
        {codes.map((c) => (
          <li key={c}>{c}</li>
        ))}
      </ol>
      <div className="flex gap-2">
        <button
          type="button"
          className="btn btn-ghost flex-1"
          onClick={async () => {
            await navigator.clipboard.writeText(text)
            setCopied(true)
          }}
        >
          {copied ? <Check size={16} /> : <Copy size={16} />} {copied ? 'Copied' : 'Copy'}
        </button>
        <a className="btn btn-ghost flex-1" href={`data:text/plain;charset=utf-8,${encodeURIComponent(text)}`} download="24cups-recovery-codes.txt">
          <Download size={16} /> Download
        </a>
      </div>
      <label className="flex items-center gap-2 text-sm">
        <input type="checkbox" checked={saved} onChange={(e) => setSaved(e.target.checked)} />
        I’ve saved these codes
      </label>
      <button type="button" className="btn btn-primary w-full" disabled={!saved} onClick={onDone}>
        {doneLabel}
      </button>
    </div>
  )
}
