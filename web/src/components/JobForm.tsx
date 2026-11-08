// Adding and changing a job. A new one can start from the posting: paste
// its link (the page's job data fills the form) or its text (Pocket AI
// reads it). The contact can be picked from Pocket Contacts when it is
// connected, typed otherwise.
import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { CURRENCIES, EMPLOYMENTS, OPEN_KINDS, PERIODS, SOURCES, WORK_MODES } from '../lib/jobs'
import { announceChange, useAppState, useData } from '../lib/data'
import { addDays, todayISO } from '../lib/format'
import type { Doc, Job, PickedContact, Posting } from '../lib/types'
import { Icon } from './icons'
import { Select } from './Select'
import { Field, Modal, Spinner, errMsg, useDebounce, useToast } from './ui'

export interface JobDraft {
  title: string; company: string; company_ref: string; stage_id: number; location: string; work_mode: string; employment: string; url: string
  min: string; max: string; currency: string; salary_period: string; source: string; excitement: number; apply_by: string; applied_on: string
  contact_name: string; contact_email: string; contact_phone: string; contact_ref: string; cv_id: number
  next_action: string; next_due: string; notes: string; posting: string; tags: string
}

export function draftOf(j?: Partial<Job>, defaults?: { currency: string; period: string; stage: number }): JobDraft {
  return {
    title: j?.title || '', company: j?.company || '', company_ref: j?.company_ref || '', stage_id: j?.stage_id || defaults?.stage || 0,
    location: j?.location || '', work_mode: j?.work_mode || '', employment: j?.employment || '', url: j?.url || '',
    min: j?.min_amount ? String(j.min_amount) : '', max: j?.max_amount ? String(j.max_amount) : '',
    currency: j?.currency || defaults?.currency || 'EUR', salary_period: j?.salary_period || defaults?.period || 'year',
    source: j?.source || '', excitement: j?.excitement || 0, apply_by: j?.apply_by || '', applied_on: j?.applied_on || '',
    contact_name: j?.contact_name || '', contact_email: j?.contact_email || '', contact_phone: j?.contact_phone || '', contact_ref: j?.contact_ref || '',
    cv_id: j?.cv_id || 0, next_action: j?.next_action || '', next_due: j?.next_due || '', notes: j?.notes || '', posting: j?.posting || '',
    tags: (j?.tags || []).join(', '),
  }
}

const num = (v: string) => Number(String(v).replace(/[\s.](?=\d{3}\b)/g, '').replace(',', '.')) || 0

function bodyOf(f: JobDraft) {
  return {
    title: f.title, company: f.company, company_ref: f.company_ref, stage_id: f.stage_id || undefined, location: f.location, work_mode: f.work_mode,
    employment: f.employment, url: f.url, min_amount: num(f.min), max_amount: num(f.max), currency: f.currency, salary_period: f.salary_period,
    source: f.source, excitement: f.excitement, apply_by: f.apply_by, applied_on: f.applied_on, contact_name: f.contact_name,
    contact_email: f.contact_email, contact_phone: f.contact_phone, contact_ref: f.contact_ref, cv_id: f.cv_id,
    next_action: f.next_action, next_due: f.next_action ? f.next_due : '', notes: f.notes, posting: f.posting,
    tags: f.tags.split(',').map(t => t.trim()).filter(Boolean),
  }
}

/** Search Pocket Contacts as you type; pick a person or a company. */
function ContactPicker({ onPick }: { onPick: (c: PickedContact) => void }) {
  const [q, setQ] = useState('')
  const dq = useDebounce(q, 250)
  const [list, setList] = useState<PickedContact[] | null>(null)
  const [off, setOff] = useState('')
  useEffect(() => {
    if (dq.trim().length < 2) { setList(null); return }
    let alive = true
    api.get<{ contacts: PickedContact[]; off?: string }>(`${V1}/apps/contacts?q=${encodeURIComponent(dq.trim())}`)
      .then(r => { if (alive) { setList(r.contacts); setOff(r.off || '') } }).catch(e => { if (alive) setOff(errMsg(e)) })
    return () => { alive = false }
  }, [dq])
  return (
    <div className="picker">
      <div className="search-in"><Icon name="search" size={15} /><input value={q} onChange={e => setQ(e.target.value)} placeholder="Find in Pocket Contacts…" aria-label="Find in Pocket Contacts" /></div>
      {off && <p className="muted small">{off}</p>}
      {list && (list.length === 0 ? <p className="muted small">Nobody called “{dq}” in your contacts. Type their details below.</p> : (
        <div className="pick-list">
          {list.map(c => (
            <button key={c.id} type="button" className="pick-row" onClick={() => { onPick(c); setQ(''); setList(null) }}>
              <span className="pick-ava" style={{ background: c.color || 'var(--accent)' }}>{c.kind === 'org' ? <Icon name="building" size={13} /> : (c.initials || c.name.slice(0, 1))}</span>
              <span className="grow ellipsis"><b>{c.name}</b>{(c.company || c.email) && <span className="muted"> · {[c.kind !== 'org' && c.company, c.email].filter(Boolean).join(' · ')}</span>}</span>
            </button>
          ))}
        </div>
      ))}
    </div>
  )
}

/** How much you want it: 0–5, as stars you tap. */
export function Stars({ value, onChange, size = 18 }: { value: number; onChange?: (v: number) => void; size?: number }) {
  return (
    <span className={`stars ${onChange ? 'edit' : ''}`} role={onChange ? 'radiogroup' : 'img'} aria-label={`${value} of 5`}>
      {[1, 2, 3, 4, 5].map(n => onChange
        ? <button key={n} type="button" className={`star ${n <= value ? 'on' : ''}`} onClick={() => onChange(n === value ? 0 : n)} aria-label={`${n} of 5`} aria-checked={n === value} role="radio"><Icon name="star" size={size} /></button>
        : <Icon key={n} name="star" size={size} className={`star ${n <= value ? 'on' : ''}`} />)}
    </span>
  )
}

/** Read a posting from its link or text into the form. */
function PostingReader({ onRead }: { onRead: (p: Posting, extra: { contact_name?: string; contact_email?: string; summary?: string }) => void }) {
  const { toast } = useToast()
  const [v, setV] = useState('')
  const [busy, setBusy] = useState(false)
  const [notes, setNotes] = useState<string[]>([])
  const isURL = /^\s*(https?:\/\/|www\.)\S+\s*$/i.test(v)
  const read = async () => {
    setBusy(true)
    setNotes([])
    try {
      const r = await api.post<{ posting: Posting; ai: boolean; notes?: string[]; contact_name?: string; contact_email?: string; summary?: string }>(`${V1}/postings/read`, isURL ? { url: v.trim() } : { text: v })
      onRead(r.posting, r)
      setNotes(r.notes || [])
      toast(r.posting.structured ? 'Filled from the page' : r.ai ? 'Read by Pocket AI — check it' : 'Filled what could be read')
      setV('')
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  return (
    <div className="reader">
      <div className="reader-h"><Icon name="sparkles" size={15} />Start from the posting</div>
      <div className="row tight">
        <textarea rows={isURL || !v ? 1 : 4} value={v} onChange={e => setV(e.target.value)} placeholder="Paste the link to the job, or its whole text" aria-label="The posting's link or text" className="grow" />
        <button type="button" className="btn" onClick={read} disabled={busy || v.trim().length < 8}>{busy ? <Spinner /> : <Icon name="download" size={15} />}Read it</button>
      </div>
      {notes.map(n => <p key={n} className="muted small"><Icon name="info" size={12} className="inline-ico" /> {n}</p>)}
    </div>
  )
}

export function JobForm({ job, initial, onClose, onSaved }: { job?: Job; initial?: Partial<Job>; onClose: () => void; onSaved?: (j: Job) => void }) {
  const { settings } = useApp()
  const { stages } = useData()
  const appState = useAppState()
  const contacts = appState('contacts', 'contacts:read')
  const navigate = useNavigate()
  const { toast } = useToast()
  const openStages = useMemo(() => (stages || []).filter(s => OPEN_KINDS.includes(s.kind)), [stages])
  const [f, setF] = useState<JobDraft>(() => draftOf(job || initial, { currency: settings?.currency || 'EUR', period: settings?.period || 'year', stage: initial?.stage_id || openStages[0]?.id || 0 }))
  const [docs, setDocs] = useState<Doc[] | null>(null)
  const [more, setMore] = useState(!!job && !!(job.notes || job.tags.length || job.source || job.apply_by || job.posting))
  const [summary, setSummary] = useState('')
  const [busy, setBusy] = useState(false)
  const titleRef = useRef<HTMLInputElement>(null)
  useEffect(() => { if (job || !initial?.url) titleRef.current?.focus() }, []) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { api.get<Doc[]>(`${V1}/docs?kind=cv`).then(setDocs).catch(() => setDocs([])) }, [])
  const set = <K extends keyof JobDraft>(k: K, v: JobDraft[K]) => setF(x => ({ ...x, [k]: v }))
  const pick = (c: PickedContact) => setF(x => c.kind === 'org'
    ? { ...x, company: c.name, company_ref: c.id }
    : { ...x, contact_name: c.name, contact_email: c.email || x.contact_email, contact_phone: c.phone || x.contact_phone, contact_ref: c.id, company: c.company || x.company })
  const fromPosting = (p: Posting, extra: { contact_name?: string; contact_email?: string; summary?: string }) => {
    setF(x => ({
      ...x, title: p.title || x.title, company: p.company || x.company, location: p.location || x.location, work_mode: p.work_mode || x.work_mode,
      employment: p.employment || x.employment, url: p.url || x.url, min: p.min_amount ? String(p.min_amount) : x.min, max: p.max_amount ? String(p.max_amount) : x.max,
      currency: p.currency && /^[A-Z]{3}$/.test(p.currency) ? p.currency : x.currency, salary_period: p.salary_period || x.salary_period,
      apply_by: p.apply_by || x.apply_by, posting: p.posting || x.posting, source: x.source || p.source || '',
      contact_name: x.contact_name || extra.contact_name || '', contact_email: x.contact_email || extra.contact_email || '',
    }))
    if (extra.summary) setSummary(extra.summary)
    if (p.posting) setMore(true)
  }
  // a link shared to /jobs/new: read it at once
  const auto = useRef(false)
  useEffect(() => {
    if (job || auto.current || !initial?.url || initial.title) return
    auto.current = true
    api.post<{ posting: Posting; notes?: string[]; contact_name?: string; contact_email?: string; summary?: string }>(`${V1}/postings/read`, { url: initial.url })
      .then(r => fromPosting(r.posting, r)).catch(() => {})
  }, []) // eslint-disable-line react-hooks/exhaustive-deps
  const save = async (e?: React.FormEvent) => {
    e?.preventDefault()
    setBusy(true)
    try {
      const body = bodyOf(f)
      const j = job ? await api.put<Job>(`${V1}/jobs/${job.id}`, body) : await api.post<Job>(`${V1}/jobs`, body)
      announceChange()
      toast(job ? 'Saved' : `Saved “${j.title || j.company}”`)
      if (onSaved) onSaved(j)
      else navigate(`/jobs/${j.id}`)
      onClose()
    } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) }
  }
  const linked = f.contact_ref || f.company_ref
  const stage = openStages.find(s => s.id === f.stage_id)
  return (
    <Modal title={job ? 'Change the job' : 'Add a job'} onClose={onClose} size="lg" footer={<>
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" onClick={() => save()} disabled={busy || !(f.title || f.company)}>{busy ? <Spinner /> : <Icon name="check" size={15} />}{job ? 'Save' : 'Save the job'}</button>
    </>}>
      <form className="stack" onSubmit={save}>
        {!job && <PostingReader onRead={fromPosting} />}
        {summary && <div className="callout ai-sum"><div className="row"><Icon name="sparkles" size={14} /><b>In short</b></div><p className="pre small">{summary}</p></div>}
        <div className="two-col tight">
          <Field label="The role"><input ref={titleRef} value={f.title} onChange={e => set('title', e.target.value)} placeholder="Backend developer" /></Field>
          <Field label="Company"><input value={f.company} onChange={e => setF({ ...f, company: e.target.value, company_ref: '' })} placeholder="Acme" autoComplete="off" /></Field>
          <Field label="Where"><input value={f.location} onChange={e => set('location', e.target.value)} placeholder="Sarajevo, or Remote (EU)" /></Field>
          <Field label="How" group><Select ariaLabel="Work mode" value={f.work_mode} onChange={v => set('work_mode', v)} placeholder="Not said" clearLabel="Not said" options={WORK_MODES} /></Field>
        </div>
        <Field label="Link to the posting"><input type="url" inputMode="url" value={f.url} onChange={e => set('url', e.target.value)} placeholder="https://…" /></Field>
        <section className="form-sec">
          <h4 className="form-sec-h">Salary</h4>
          <div className="salary-row">
            <input inputMode="decimal" value={f.min} onChange={e => set('min', e.target.value)} placeholder="From" aria-label="Salary from" />
            <span className="muted">–</span>
            <input inputMode="decimal" value={f.max} onChange={e => set('max', e.target.value)} placeholder="To" aria-label="Salary to" />
            <Select ariaLabel="Currency" value={f.currency} onChange={v => set('currency', v)} options={Array.from(new Set([f.currency, ...CURRENCIES])).map(c => ({ value: c, label: c }))} />
            <Select ariaLabel="Per" value={f.salary_period} onChange={v => set('salary_period', v)} options={PERIODS.map(p => ({ value: p.value, label: p.label }))} />
          </div>
        </section>
        <div className="two-col tight">
          {!job && <Field label="Stage" group help={stage?.kind === 'applied' ? 'Counted as applied today.' : undefined}>
            <Select ariaLabel="Stage" value={String(f.stage_id || '')} onChange={v => set('stage_id', Number(v))} options={openStages.map(s => ({ value: String(s.id), label: s.name }))} />
          </Field>}
          {job && <Field label="Applied on" help="Empty: not yet."><input type="date" value={f.applied_on} onChange={e => set('applied_on', e.target.value)} /></Field>}
          <Field label="How much you want it" group><Stars value={f.excitement} onChange={v => set('excitement', v)} /></Field>
        </div>
        <section className="form-sec">
          <h4 className="form-sec-h">Who to talk to{linked && <span className="badge ok"><Icon name="link" size={11} />In Pocket Contacts</span>}</h4>
          {contacts.ok ? <ContactPicker onPick={pick} /> : contacts.app && <p className="muted small"><Icon name="info" size={12} className="inline-ico" /> Type their details. {contacts.app.state === 'missing' ? 'With Pocket Contacts on this box you could pick them from your contacts.' : 'Connect Pocket Contacts (Settings → Connected apps) to pick them from your contacts.'}</p>}
          <div className="three-col tight">
            <Field label="Recruiter or manager"><input value={f.contact_name} onChange={e => setF({ ...f, contact_name: e.target.value, contact_ref: '' })} placeholder="Ana Marić" autoComplete="off" /></Field>
            <Field label="Email"><input type="email" value={f.contact_email} onChange={e => set('contact_email', e.target.value)} autoComplete="off" /></Field>
            <Field label="Phone"><input type="tel" value={f.contact_phone} onChange={e => set('contact_phone', e.target.value)} autoComplete="off" /></Field>
          </div>
        </section>
        <section className="form-sec">
          <h4 className="form-sec-h">Next step</h4>
          <div className="two-col tight next-row">
            <Field label="What you do next"><input value={f.next_action} onChange={e => set('next_action', e.target.value)} placeholder="Tailor the CV and apply" /></Field>
            <Field label="When" group>
              <div className="row tight">
                <input type="date" value={f.next_due} onChange={e => set('next_due', e.target.value)} disabled={!f.next_action} aria-label="When" />
                {!f.next_due && f.next_action && <button type="button" className="btn sm" onClick={() => set('next_due', addDays(todayISO(), settings?.action_days || 3))}>In {settings?.action_days || 3} days</button>}
              </div>
            </Field>
          </div>
        </section>
        {!more && <button type="button" className="link-btn self-start" onClick={() => setMore(true)}><Icon name="plus" size={14} />The posting’s text, your notes, the CV you send, where you found it…</button>}
        {more && <section className="form-sec">
          <div className="two-col tight">
            <Field label="CV for this job" group help={docs && !docs.length ? 'Add your CV in CVs & letters.' : undefined}>
              <Select ariaLabel="CV" value={String(f.cv_id || '')} onChange={v => set('cv_id', Number(v))} placeholder="Your main CV" clearLabel="Your main CV" options={(docs || []).map(d => ({ value: String(d.id), label: d.name, hint: d.is_default ? 'main' : undefined }))} />
            </Field>
            <Field label="Found it on" group><Select ariaLabel="Found it on" value={f.source} onChange={v => set('source', v)} placeholder="Not said" clearLabel="Not said" options={SOURCES} /></Field>
            <Field label="Apply by"><input type="date" value={f.apply_by} onChange={e => set('apply_by', e.target.value)} /></Field>
            <Field label="Kind of job" group><Select ariaLabel="Kind of job" value={f.employment} onChange={v => set('employment', v)} placeholder="Not said" clearLabel="Not said" options={EMPLOYMENTS} /></Field>
            <Field label="Tags" help="Separated by commas."><input value={f.tags} onChange={e => set('tags', e.target.value)} placeholder="remote, payments" /></Field>
          </div>
          <Field label="Your notes" help="Why it is interesting, what to ask, who you know there."><textarea rows={3} value={f.notes} onChange={e => set('notes', e.target.value)} /></Field>
          <Field label="The posting" help="Its text: Pocket AI uses it to check the fit, write a cover letter and prepare you."><textarea rows={6} value={f.posting} onChange={e => set('posting', e.target.value)} /></Field>
        </section>}
      </form>
    </Modal>
  )
}
