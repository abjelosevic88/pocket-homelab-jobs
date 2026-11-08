// Bring jobs in: a Pocket Jobs export, or a CSV from Huntr, Teal, Simplify
// or a spreadsheet. Bringing the same file again adds nothing twice.
import { useState } from 'react'
import { V1 } from '../lib/api'
import { announceChange } from '../lib/data'
import { Icon } from './icons'
import { Modal, Spinner, errMsg, useToast } from './ui'

export function ImportModal({ onClose }: { onClose: () => void }) {
  const { toast } = useToast()
  const [busy, setBusy] = useState(false)
  const [done, setDone] = useState<{ jobs: number; skipped: number; docs?: number } | null>(null)
  const send = async (file: File) => {
    setBusy(true)
    try {
      const fd = new FormData()
      fd.append('file', file)
      const res = await fetch(`${V1}/import`, { method: 'POST', body: fd, credentials: 'same-origin' })
      const j = await res.json()
      if (!res.ok) throw new Error(j.error || res.statusText)
      setDone(j)
      announceChange()
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title="Bring your jobs" onClose={onClose} size="sm" footer={<button type="button" className="btn" onClick={onClose}>{done ? 'Done' : 'Cancel'}</button>}>
      {done ? <p><Icon name="check-circle" size={16} className="inline-ico ok-text" /> {done.jobs} {done.jobs === 1 ? 'job' : 'jobs'} added{done.docs ? `, ${done.docs} CVs and letters` : ''}{done.skipped ? `, ${done.skipped} already here` : ''}.</p> : (
        <div className="stack">
          <p className="muted">A CSV file with one job per row: exported from Huntr, Teal, Simplify, Excel or Google Sheets (File → Download → CSV), or a Pocket Jobs export.</p>
          <p className="muted small">Columns it understands: job title or position, company, status or list (Wishlist, Applied, Interviewing, Offer, Rejected…), location, URL, min and max salary, currency, excitement, date applied, deadline, contact, email, notes, tags. Stages it does not know are added to your board.</p>
          <label className="btn primary file-btn">{busy ? <Spinner /> : <Icon name="upload" size={15} />}Choose a file<input type="file" accept=".csv,.tsv,.txt,.json,text/csv,application/json" hidden onChange={e => { const f = e.target.files?.[0]; if (f) send(f) }} disabled={busy} /></label>
          <p className="muted small">Bringing the same file again adds nothing twice.</p>
        </div>
      )}
    </Modal>
  )
}
