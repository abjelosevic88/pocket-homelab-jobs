// The sheets behind a job's buttons: a step done, a line on the timeline,
// closing it or getting hired, an interview, an offer, and making something
// in another app — with a way that works without that app (a calendar
// file, your own email app) when it is not there.
import { useEffect, useState } from 'react'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { APP_INFO, CURRENCIES, END_REASONS, INTERVIEW_KINDS, LOG_KINDS, OUTCOMES, PERIODS, TARGET_APP, jobLabel } from '../lib/jobs'
import { announceChange, useAppState, useData } from '../lib/data'
import { addDays, todayISO } from '../lib/format'
import type { ActivityKind, Interview, Job, Made, Offer, QA } from '../lib/types'
import { AppOff } from './Apps'
import { Icon } from './icons'
import { Select } from './Select'
import { Field, Modal, Segmented, Spinner, Switch, errMsg, useToast } from './ui'

/** Whether Pocket AI (or the person's own key) answers, and AI help is on. */
export function useAI() {
  const { settings } = useApp()
  const [ready, setReady] = useState(false)
  useEffect(() => {
    if (!settings?.ai_help) return
    api.get<{ ready: boolean }>(`${V1}/ai/status`).then(r => setReady(r.ready)).catch(() => setReady(false))
  }, [settings?.ai_help])
  return ready && !!settings?.ai_help
}

// ---- the next step ----

export function StepSheet({ job, onClose, onDone }: { job: Job; onClose: () => void; onDone: (j: Job) => void }) {
  const { settings } = useApp()
  const { toast } = useToast()
  const ai = useAI()
  const had = !!job.next_action
  const [kind, setKind] = useState<ActivityKind>('step')
  const [note, setNote] = useState('')
  const [next, setNext] = useState(had ? '' : job.next_action)
  const [due, setDue] = useState(addDays(todayISO(), settings?.action_days || 3))
  const [why, setWhy] = useState('')
  const [busy, setBusy] = useState(false)
  const [thinking, setThinking] = useState(false)
  const suggest = async () => {
    setThinking(true)
    try {
      const r = await api.post<{ step: string; due: string; why: string }>(`${V1}/jobs/${job.id}/ai`, { want: 'next' })
      setNext(r.step); setDue(r.due); setWhy(r.why)
    } catch (e) { toast(errMsg(e), 'err') } finally { setThinking(false) }
  }
  const go = async (e?: React.FormEvent) => {
    e?.preventDefault()
    setBusy(true)
    try {
      const j = had
        ? await api.post<Job>(`${V1}/jobs/${job.id}/step/done`, { kind, note, next_action: next, next_due: next ? due : '' })
        : await api.put<Job>(`${V1}/jobs/${job.id}`, { next_action: next, next_due: next ? due : '' })
      announceChange()
      toast(had ? (next ? 'Done — next step set' : 'Done') : 'Next step set')
      onDone(j)
    } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={had ? 'Step done' : 'Set the next step'} onClose={onClose} size="sm" footer={<>
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" onClick={() => go()} disabled={busy || (!had && !next)}>{busy ? <Spinner /> : <Icon name="check" size={15} />}{had ? 'Save' : 'Set it'}</button>
    </>}>
      <form className="stack" onSubmit={go}>
        {had && <>
          <p className="step-done-what"><Icon name="check-circle" size={16} />{job.next_action}</p>
          <Segmented size="sm" label="What it was" value={kind} onChange={setKind} options={[{ value: 'step', label: 'Done' }, ...LOG_KINDS.filter(k => k.value !== 'note').map(k => ({ value: k.value, label: k.label }))]} />
          <Field label="How it went" help="Optional; it goes on the timeline."><textarea rows={3} value={note} onChange={e => setNote(e.target.value)} placeholder="Sent it with the one-page CV." /></Field>
        </>}
        <Field label={had ? 'Then what' : 'What you do next'} help={why || (had ? 'Empty: no next step for now.' : undefined)} group>
          <div className="row tight">
            <input value={next} onChange={e => setNext(e.target.value)} placeholder="Follow up with the recruiter" autoFocus={!had} aria-label="Next step" />
            {ai && <button type="button" className="btn" onClick={suggest} disabled={thinking} title="Pocket AI suggests a next step from the timeline">{thinking ? <Spinner /> : <Icon name="sparkles" size={15} />}<span className="hide-sm">Suggest</span></button>}
          </div>
        </Field>
        <Field label="When"><input type="date" value={due} onChange={e => setDue(e.target.value)} disabled={!next} /></Field>
      </form>
    </Modal>
  )
}

// ---- a line on the timeline ----

export function LogSheet({ job, onClose, onDone }: { job: Job; onClose: () => void; onDone: () => void }) {
  const { toast } = useToast()
  const [kind, setKind] = useState<ActivityKind>('note')
  const [title, setTitle] = useState('')
  const [body, setBody] = useState('')
  const [when, setWhen] = useState('')
  const [busy, setBusy] = useState(false)
  const go = async (e?: React.FormEvent) => {
    e?.preventDefault()
    setBusy(true)
    try {
      await api.post(`${V1}/jobs/${job.id}/activities`, { kind, title, body, at: when })
      announceChange()
      onDone()
    } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) }
  }
  const ph: Record<string, string> = { note: 'What to remember', call: 'The recruiter called', email: 'They wrote back' }
  return (
    <Modal title="Write it down" onClose={onClose} size="sm" footer={<>
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" onClick={() => go()} disabled={busy || !(title || body)}>{busy ? <Spinner /> : <Icon name="check" size={15} />}Add</button>
    </>}>
      <form className="stack" onSubmit={go}>
        <Segmented size="sm" label="What" value={kind} onChange={setKind} options={LOG_KINDS.map(k => ({ value: k.value, label: k.label }))} />
        <Field label="In a few words"><input value={title} onChange={e => setTitle(e.target.value)} placeholder={ph[kind]} autoFocus /></Field>
        <Field label="More" help="Optional."><textarea rows={4} value={body} onChange={e => setBody(e.target.value)} /></Field>
        <Field label="When" help="Empty: now."><input type="datetime-local" value={when} onChange={e => setWhen(e.target.value)} /></Field>
      </form>
    </Modal>
  )
}

// ---- closed, hired ----

export function CloseSheet({ job, onClose, onDone }: { job: Job; onClose: () => void; onDone: (j: Job) => void }) {
  const { stages } = useData()
  const { toast } = useToast()
  const closed = (stages || []).filter(s => s.kind === 'closed')
  const [stage, setStage] = useState(closed[0]?.id || 0)
  const [reason, setReason] = useState(job.applied_on ? 'rejected' : 'other')
  const [note, setNote] = useState('')
  const [busy, setBusy] = useState(false)
  const go = async () => {
    setBusy(true)
    try {
      const j = await api.post<Job>(`${V1}/jobs/${job.id}/move`, { stage_id: stage, end_reason: reason, note })
      announceChange()
      toast('Closed')
      onDone(j)
    } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title="Close this job" onClose={onClose} size="sm" footer={<>
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" onClick={go} disabled={busy || !stage}>{busy ? <Spinner /> : <Icon name="x-circle" size={15} />}Close it</button>
    </>}>
      <div className="stack">
        {!closed.length && <p className="warn-text">There is no “closed” stage. Add one in Settings → Stages.</p>}
        {closed.length > 1 && <Field label="Stage" group><Select ariaLabel="Stage" value={String(stage)} onChange={v => setStage(Number(v))} options={closed.map(s => ({ value: String(s.id), label: s.name }))} /></Field>}
        <Field label="Why" help="How it goes shows why jobs end — useful to see a pattern." group>
          <div className="chips">{END_REASONS.map(r => <button key={r.value} type="button" className={`chip ${reason === r.value ? 'on' : ''}`} onClick={() => setReason(r.value)}>{r.label}</button>)}</div>
        </Field>
        <Field label="Anything to remember" help="Optional: what they said, what you would do differently."><textarea rows={3} value={note} onChange={e => setNote(e.target.value)} placeholder="They went with someone with more Kubernetes." /></Field>
      </div>
    </Modal>
  )
}

export function HiredSheet({ job, onClose, onDone }: { job: Job; onClose: () => void; onDone: (j: Job) => void }) {
  const { stages } = useData()
  const { toast } = useToast()
  const hired = (stages || []).find(s => s.kind === 'hired')
  const [note, setNote] = useState('')
  const [busy, setBusy] = useState(false)
  const go = async () => {
    if (!hired) return
    setBusy(true)
    try {
      const j = await api.post<Job>(`${V1}/jobs/${job.id}/move`, { stage_id: hired.id, note })
      announceChange()
      toast('Hired 🎉 Congratulations!')
      onDone(j)
    } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title="Hired 🎉" onClose={onClose} size="sm" footer={<>
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" onClick={go} disabled={busy || !hired}>{busy ? <Spinner /> : <Icon name="trophy" size={15} />}I took the job</button>
    </>}>
      <div className="stack">
        {!hired && <p className="warn-text">There is no “hired” stage. Add one in Settings → Stages.</p>}
        <p><b>{jobLabel(job)}</b> — well done.</p>
        <Field label="What was agreed" help="Optional: the start day, the pay, anything to remember."><textarea rows={3} value={note} onChange={e => setNote(e.target.value)} autoFocus /></Field>
        <p className="muted small">The other jobs stay as they are; close them when you are ready (or tell them kindly with an email from their page).</p>
      </div>
    </Modal>
  )
}

// ---- an interview ----

export function InterviewSheet({ job, interview, onClose, onDone }: { job: Pick<Job, 'id' | 'title' | 'company' | 'location' | 'contact_name'>; interview?: Interview; onClose: () => void; onDone: (iv: Interview) => void }) {
  const { settings } = useApp()
  const appState = useAppState()
  const cal = appState('calendar', 'calendar:write')
  const { toast } = useToast()
  const [f, setF] = useState({
    kind: interview?.kind || 'video', date: interview?.starts_at.slice(0, 10) || addDays(todayISO(), 2), time: interview && interview.starts_at.length > 10 ? interview.starts_at.slice(11, 16) : '10:00',
    minutes: interview?.minutes || settings?.interview_minutes || 45, location: interview?.location || '', with_people: interview?.with_people || job.contact_name || '',
    prep: interview?.prep || '', outcome: interview?.outcome || '', feedback: interview?.feedback || '', thanks_sent: !!interview?.thanks_sent,
  })
  const [toCal, setToCal] = useState(!!settings?.calendar_sync)
  const [busy, setBusy] = useState(false)
  const past = !!interview?.past
  const go = async (e?: React.FormEvent) => {
    e?.preventDefault()
    setBusy(true)
    try {
      const body = { ...f, calendar: !interview && cal.ok ? toCal : undefined }
      const r = interview ? await api.put<{ interview: Interview; calendar_error?: string }>(`${V1}/interviews/${interview.id}`, body)
        : await api.post<{ interview: Interview; calendar_error?: string }>(`${V1}/jobs/${job.id}/interviews`, body)
      announceChange()
      if (r.calendar_error) toast(r.calendar_error, 'err')
      else toast(interview ? 'Saved' : r.interview.event_ref ? 'Planned — it is in your calendar' : 'Planned')
      onDone(r.interview)
    } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) }
  }
  const remote = ['phone', 'video', 'take_home'].includes(f.kind)
  return (
    <Modal title={interview ? 'The interview' : 'Plan an interview'} onClose={onClose} footer={<>
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" onClick={() => go()} disabled={busy || !f.date}>{busy ? <Spinner /> : <Icon name="check" size={15} />}{interview ? 'Save' : 'Plan it'}</button>
    </>}>
      <form className="stack" onSubmit={go}>
        <p className="muted small">{jobLabel(job)}</p>
        <div className="kind-grid" role="radiogroup" aria-label="Kind">
          {INTERVIEW_KINDS.map(k => <button key={k.value} type="button" role="radio" aria-checked={f.kind === k.value} className={`kind-btn ${f.kind === k.value ? 'on' : ''}`} onClick={() => setF({ ...f, kind: k.value })}><Icon name={k.icon} size={15} />{k.label}</button>)}
        </div>
        <div className="three-col tight">
          <Field label={f.kind === 'take_home' ? 'Due' : 'Day'}><input type="date" value={f.date} onChange={e => setF({ ...f, date: e.target.value })} required /></Field>
          <Field label="Time" help="Empty: the whole day."><input type="time" value={f.time} onChange={e => setF({ ...f, time: e.target.value })} /></Field>
          <Field label="Minutes"><input type="number" min={5} max={600} step={5} value={f.minutes} onChange={e => setF({ ...f, minutes: Number(e.target.value) || 45 })} /></Field>
        </div>
        <Field label={remote ? 'Call link' : 'Address'}><input value={f.location} onChange={e => setF({ ...f, location: e.target.value })} placeholder={remote ? 'https://meet…' : job.location || 'Street, town'} /></Field>
        <Field label="With"><input value={f.with_people} onChange={e => setF({ ...f, with_people: e.target.value })} placeholder="The hiring manager and a developer" /></Field>
        {!interview && <div className={`won-step ${cal.ok ? '' : 'off'}`}>
          <Switch checked={cal.ok && toCal} onChange={setToCal} label="Put it in Pocket Calendar" disabled={!cal.ok} />
          <div className="grow"><div className="won-step-h"><Icon name="calendar" size={14} />Put it in Pocket Calendar</div>
            <div className="muted small">{cal.ok ? 'It moves there when you move it here.' : `Not connected${cal.app?.fix ? ' — ' + cal.app.fix : ''}. You can download it for any calendar from the job’s page.`}</div></div>
        </div>}
        {past || interview ? <>
          <Field label="How it went" group><Segmented size="sm" label="How it went" value={f.outcome} onChange={v => setF({ ...f, outcome: v })} options={[{ value: '' as const, label: 'Not yet' }, ...OUTCOMES]} /></Field>
          <Field label="Notes after" help="What they asked, what you said, what comes next."><textarea rows={3} value={f.feedback} onChange={e => setF({ ...f, feedback: e.target.value })} /></Field>
          <Switch checked={f.thanks_sent} onChange={v => setF({ ...f, thanks_sent: v })} label="Thank-you sent" />
        </> : null}
        <Field label="Preparation" help="What to get across, what to check before. Pocket AI can write this from the posting and your CV on the job’s page."><textarea rows={3} value={f.prep} onChange={e => setF({ ...f, prep: e.target.value })} /></Field>
      </form>
    </Modal>
  )
}

/** The questions to practise for an interview, editable, with AI help. */
export function QuestionsSheet({ job, interview, onClose, onDone }: { job: Job; interview: Interview; onClose: () => void; onDone: () => void }) {
  const { toast } = useToast()
  const ai = useAI()
  const appState = useAppState()
  const learn = appState('learn', 'learn:write')
  const notes = appState('notes', 'notes:write')
  const [qs, setQs] = useState<QA[]>(interview.questions.length ? interview.questions : [{ q: '', a: '' }])
  const [prep, setPrep] = useState(interview.prep)
  const [busy, setBusy] = useState('')
  const save = async (quiet = false) => {
    await api.put(`${V1}/interviews/${interview.id}`, { questions: qs.filter(q => q.q.trim()), prep })
    announceChange()
    if (!quiet) toast('Saved')
  }
  const suggest = async () => {
    setBusy('ai')
    try {
      const r = await api.post<{ prep: string; questions: QA[] }>(`${V1}/jobs/${job.id}/ai`, { want: 'prep', interview_id: interview.id })
      const have = new Set(qs.map(q => q.q.trim().toLowerCase()))
      setQs(x => [...x.filter(q => q.q.trim()), ...r.questions.filter(q => !have.has(q.q.trim().toLowerCase()))])
      setPrep(p => p ? p + '\n\n' + r.prep : r.prep)
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy('') }
  }
  const to = async (what: 'cards' | 'note') => {
    setBusy(what)
    try {
      await save(true)
      const r = await api.post<{ made?: number; deck?: string; url?: string; title?: string }>(`${V1}/interviews/${interview.id}/${what}`)
      toast(what === 'cards' ? `${r.made} flashcards in “${r.deck}”` : 'Saved to Pocket Notes')
      announceChange()
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy('') }
  }
  return (
    <Modal title="Prepare" onClose={onClose} size="lg" footer={<>
      <button type="button" className="btn" disabled={!!busy || !learn.ok} onClick={() => to('cards')} title={learn.ok ? 'Practise them as flashcards in Pocket Learn' : 'Pocket Learn is not connected'}>{busy === 'cards' ? <Spinner /> : <Icon name="brain" size={15} />}<span className="hide-sm">To Pocket Learn</span></button>
      <button type="button" className="btn" disabled={!!busy || !notes.ok} onClick={() => to('note')} title={notes.ok ? 'Save the preparation as a note' : 'Pocket Notes is not connected'}>{busy === 'note' ? <Spinner /> : <Icon name="note" size={15} />}<span className="hide-sm">To Pocket Notes</span></button>
      <span className="spacer" />
      <button type="button" className="btn" onClick={onClose}>Close</button>
      <button type="button" className="btn primary" onClick={async () => { await save(); onDone() }} disabled={!!busy}><Icon name="check" size={15} />Save</button>
    </>}>
      <div className="stack">
        <div className="row">
          <p className="muted small grow">{jobLabel(job)} · {INTERVIEW_KINDS.find(k => k.value === interview.kind)?.label}</p>
          {ai && <button type="button" className="btn sm" onClick={suggest} disabled={!!busy}>{busy === 'ai' ? <Spinner /> : <Icon name="sparkles" size={14} />}Suggest questions</button>}
        </div>
        <Field label="Preparation"><textarea rows={4} value={prep} onChange={e => setPrep(e.target.value)} placeholder="What to get across, what to check before" /></Field>
        <h4 className="form-sec-h">Questions to practise</h4>
        {qs.map((q, i) => (
          <div key={i} className="qa">
            <div className="row tight">
              <input value={q.q} onChange={e => setQs(x => x.map((y, k) => k === i ? { ...y, q: e.target.value } : y))} placeholder="Tell us about a project you are proud of" aria-label={`Question ${i + 1}`} />
              <button type="button" className="btn ghost icon-only sm" onClick={() => setQs(x => x.filter((_, k) => k !== i))} aria-label="Remove"><Icon name="x" size={14} /></button>
            </div>
            <textarea rows={2} value={q.a} onChange={e => setQs(x => x.map((y, k) => k === i ? { ...y, a: e.target.value } : y))} placeholder="Notes for your answer" aria-label={`Answer ${i + 1}`} />
          </div>
        ))}
        <button type="button" className="link-btn self-start" onClick={() => setQs(x => [...x, { q: '', a: '' }])}><Icon name="plus" size={14} />A question</button>
        {!learn.ok && learn.app && <AppOff slug="learn" app={learn.app} compact />}
      </div>
    </Modal>
  )
}

// ---- an offer ----

export function OfferSheet({ job, offer, onClose, onDone }: { job: Job; offer: Offer | null; onClose: () => void; onDone: (o: Offer) => void }) {
  const { toast } = useToast()
  const [f, setF] = useState({
    base_amount: offer?.base_amount ? String(offer.base_amount) : '', currency: offer?.currency || job.currency, period: offer?.period || job.salary_period,
    bonus_amount: offer?.bonus_amount ? String(offer.bonus_amount) : '', equity: offer?.equity || '', pto_days: offer?.pto_days ? String(offer.pto_days) : '',
    benefits: offer?.benefits || '', start_date: offer?.start_date || '', decide_by: offer?.decide_by || '', status: offer?.status || 'open', notes: offer?.notes || '',
  })
  const [busy, setBusy] = useState(false)
  const n = (v: string) => Number(String(v).replace(/[\s.](?=\d{3}\b)/g, '').replace(',', '.')) || 0
  const go = async (e?: React.FormEvent) => {
    e?.preventDefault()
    setBusy(true)
    try {
      const o = await api.put<Offer>(`${V1}/jobs/${job.id}/offer`, { ...f, base_amount: n(f.base_amount), bonus_amount: n(f.bonus_amount), pto_days: Math.round(n(f.pto_days)) })
      announceChange()
      toast(offer ? 'Saved' : 'Offer added 🎉')
      onDone(o)
    } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={offer ? 'The offer' : 'Add the offer'} onClose={onClose} footer={<>
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" onClick={() => go()} disabled={busy}>{busy ? <Spinner /> : <Icon name="check" size={15} />}Save</button>
    </>}>
      <form className="stack" onSubmit={go}>
        <p className="muted small">{jobLabel(job)}</p>
        <Field label="Pay" group>
          <div className="salary-row">
            <input inputMode="decimal" value={f.base_amount} onChange={e => setF({ ...f, base_amount: e.target.value })} placeholder="0" aria-label="Pay" autoFocus />
            <Select ariaLabel="Currency" value={f.currency} onChange={v => setF({ ...f, currency: v })} options={Array.from(new Set([f.currency, ...CURRENCIES])).map(c => ({ value: c, label: c }))} />
            <Select ariaLabel="Per" value={f.period} onChange={v => setF({ ...f, period: v as Offer['period'] })} options={PERIODS.map(p => ({ value: p.value, label: p.label }))} />
          </div>
        </Field>
        <div className="three-col tight">
          <Field label="Bonus a year"><input inputMode="decimal" value={f.bonus_amount} onChange={e => setF({ ...f, bonus_amount: e.target.value })} placeholder="0" /></Field>
          <Field label="Days off"><input inputMode="numeric" value={f.pto_days} onChange={e => setF({ ...f, pto_days: e.target.value })} placeholder="25" /></Field>
          <Field label="Shares or options"><input value={f.equity} onChange={e => setF({ ...f, equity: e.target.value })} placeholder="None" /></Field>
        </div>
        <Field label="What else comes with it"><textarea rows={2} value={f.benefits} onChange={e => setF({ ...f, benefits: e.target.value })} placeholder="Health insurance, a learning budget, a laptop…" /></Field>
        <div className="two-col tight">
          <Field label="Start"><input type="date" value={f.start_date} onChange={e => setF({ ...f, start_date: e.target.value })} /></Field>
          <Field label="Decide by"><input type="date" value={f.decide_by} onChange={e => setF({ ...f, decide_by: e.target.value })} /></Field>
        </div>
        {offer && <Field label="Where it stands" group><Segmented size="sm" label="Where it stands" value={f.status} onChange={v => setF({ ...f, status: v })} options={[{ value: 'open', label: 'To decide' }, { value: 'negotiating', label: 'Negotiating' }, { value: 'accepted', label: 'Accepted' }, { value: 'declined', label: 'Declined' }]} /></Field>}
        {offer && f.status !== offer.status && f.status === 'accepted' && <p className="muted small"><Icon name="trophy" size={12} className="inline-ico" /> The job moves to Hired.</p>}
        {offer && f.status !== offer.status && f.status === 'declined' && <p className="muted small"><Icon name="info" size={12} className="inline-ico" /> The job is closed (you withdrew).</p>}
        <Field label="Notes"><textarea rows={2} value={f.notes} onChange={e => setF({ ...f, notes: e.target.value })} /></Field>
      </form>
    </Modal>
  )
}

// ---- in another app ----

const TITLES: Record<string, string> = { task: 'Make a task', note: 'Save as a note', email: 'Send an email', contact: 'Link to Pocket Contacts' }

export const MAKE_SCOPE: Record<string, string> = { contact: 'contacts:read', task: 'tasks:write', note: 'notes:write', email: 'mail:send' }

export const EMAIL_KINDS: { value: string; label: string }[] = [
  { value: 'follow_up', label: 'Follow up' }, { value: 'thank_you', label: 'Thank you' }, { value: 'ask', label: 'A question' },
  { value: 'accept', label: 'Accept' }, { value: 'decline', label: 'Decline' }, { value: 'withdraw', label: 'Withdraw' },
]

export function MakeSheet({ job, target, emailKind = 'follow_up', interviewID = 0, onClose, onDone }: { job: Job; target: string; emailKind?: string; interviewID?: number; onClose: () => void; onDone: (m?: Made) => void }) {
  const { settings } = useApp()
  const appState = useAppState()
  const { toast } = useToast()
  const ai = useAI()
  const slug = TARGET_APP[target]
  const st = appState(slug, MAKE_SCOPE[target])
  const [kind, setKind] = useState(emailKind)
  const [f, setF] = useState({
    title: target === 'task' ? (job.next_action || `Follow up on ${jobLabel(job)}`) : target === 'note' ? jobLabel(job) : '',
    notes: '', due: job.next_due || addDays(todayISO(), settings?.action_days || 3), to: job.contact_email, subject: jobLabel(job), body: '', timeline: true,
  })
  const [busy, setBusy] = useState(false)
  const [drafting, setDrafting] = useState(false)
  const draft = async () => {
    setDrafting(true)
    try {
      const r = await api.post<{ subject: string; body: string }>(`${V1}/jobs/${job.id}/ai`, { want: 'email', kind, hint: f.body, interview_id: interviewID })
      setF(x => ({ ...x, subject: r.subject || x.subject, body: r.body }))
    } catch (e) { toast(errMsg(e), 'err') } finally { setDrafting(false) }
  }
  const go = async (e?: React.FormEvent) => {
    e?.preventDefault()
    setBusy(true)
    try {
      const m = await api.post<Made>(`${V1}/jobs/${job.id}/make`, { target, ...f })
      if (target === 'email' && interviewID && kind === 'thank_you') await api.put(`${V1}/interviews/${interviewID}`, { thanks_sent: true }).catch(() => {})
      announceChange()
      toast(target === 'email' ? 'Sent' : m.title ? `Done: ${m.title}` : 'Done')
      onDone(m)
    } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) }
  }
  const mailto = `mailto:${encodeURIComponent(f.to)}?subject=${encodeURIComponent(f.subject)}&body=${encodeURIComponent(f.body)}`
  const logEmail = async () => {
    await api.post(`${V1}/jobs/${job.id}/activities`, { kind: 'email', title: `Email: ${f.subject}`, body: f.body }).catch(() => {})
    if (interviewID && kind === 'thank_you') await api.put(`${V1}/interviews/${interviewID}`, { thanks_sent: true }).catch(() => {})
    announceChange()
    onDone()
  }
  const fallback = !st.ok && target === 'email'
  const canGo = st.ok && !busy && !(target === 'email' && (!f.to || !f.body))
  return (
    <Modal title={TITLES[target]} onClose={onClose} size={target === 'email' ? '' : 'sm'} footer={<>
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      {fallback && <a className="btn primary" href={mailto} onClick={() => setTimeout(logEmail, 300)}><Icon name="external" size={15} />Open in your email app</a>}
      {!fallback && <button type="button" className="btn primary" onClick={() => go()} disabled={!canGo}>{busy ? <Spinner /> : <Icon name={APP_INFO[slug].icon} size={15} />}{target === 'email' ? 'Send' : 'Make it'}</button>}
    </>}>
      <form className="stack" onSubmit={go}>
        {!st.ok && <AppOff slug={slug} app={st.app}>{fallback ? 'You can still write it here and send it from your own email app; it goes on the timeline too.' : 'The job does not need it: everything stays here.'}</AppOff>}
        {(target === 'task' || target === 'note') && <Field label="Title"><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} autoFocus /></Field>}
        {target === 'task' && <Field label="Due" help="Empty: no due day."><input type="date" value={f.due} onChange={e => setF({ ...f, due: e.target.value })} /></Field>}
        {target === 'note' && <Switch checked={f.timeline} onChange={v => setF({ ...f, timeline: v })} label="With the posting and the timeline" />}
        {target === 'email' && <>
          {ai && <Segmented size="sm" label="Kind of email" value={kind} onChange={setKind} className="scroll-x" options={EMAIL_KINDS} />}
          <Field label="To"><input type="email" value={f.to} onChange={e => setF({ ...f, to: e.target.value })} placeholder="recruiter@company.com" /></Field>
          <Field label="Subject"><input value={f.subject} onChange={e => setF({ ...f, subject: e.target.value })} /></Field>
          <Field label="Message" group help={ai ? 'Write what it should say and let Pocket AI word it, or write it all yourself.' : undefined}>
            <textarea rows={9} value={f.body} onChange={e => setF({ ...f, body: e.target.value })} placeholder={`Hi ${job.contact_name.split(' ')[0] || ''},`} autoFocus />
            {ai && <div className="row"><button type="button" className="btn sm" onClick={draft} disabled={drafting}>{drafting ? <Spinner /> : <Icon name="sparkles" size={14} />}{f.body ? 'Word it for me' : 'Write it for me'}</button></div>}
          </Field>
        </>}
        {target === 'task' && <Field label="Notes" help="Optional. A link back to the job is added."><textarea rows={2} value={f.notes} onChange={e => setF({ ...f, notes: e.target.value })} /></Field>}
      </form>
    </Modal>
  )
}
