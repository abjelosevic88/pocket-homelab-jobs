// One job: where it stands, the next step, the interviews, the offer, what
// Pocket AI makes of it (fit, cover letter, CV changes), the posting, the
// timeline, and what the other apps know about the company.
import { useCallback, useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { ACTIVITY, APP_INFO, EMPLOYMENTS, OPEN_KINDS, TARGET_APP, TARGET_LABEL, dayWords, endLabel, interviewKind, jobLabel, modeLabel, salaryText, sourceLabel, stageColor, whenText } from '../lib/jobs'
import { announceChange, useAppState, useData, useOnChange } from '../lib/data'
import { ago, fmtDate, fmtDateTime, fmtMoney, todayISO } from '../lib/format'
import type { Activity, ElseSection, Interview, Job, JobView, Made, Offer } from '../lib/types'
import { AppOff, offTitle } from '../components/Apps'
import { JobForm, Stars } from '../components/JobForm'
import { CloseSheet, HiredSheet, InterviewSheet, LogSheet, MAKE_SCOPE, MakeSheet, OfferSheet, QuestionsSheet, StepSheet, useAI } from '../components/JobActions'
import { Icon } from '../components/icons'
import { useCrumb } from '../components/Layout'
import { Confirm, Empty, Loading, type MenuItem, MenuButton, Modal, Spinner, copyText, errMsg, useToast } from '../components/ui'

export default function JobPage() {
  const { id } = useParams()
  const navigate = useNavigate()
  const { toast } = useToast()
  const [view, setView] = useState<JobView | null>(null)
  const [err, setErr] = useState('')
  const load = useCallback(async () => {
    try { setView(await api.get<JobView>(`${V1}/jobs/${id}`)); setErr('') } catch (e) { setErr(errMsg(e)) }
  }, [id])
  useEffect(() => { setView(null); load() }, [load])
  useOnChange(load)
  // a next-step task may have been ticked off in Pocket Tasks: look once when the page opens
  useEffect(() => {
    if (!id) return
    api.post<Made[]>(`${V1}/jobs/${id}/sync`).then(made => setView(v => v ? { ...v, made } : v)).catch(() => {})
  }, [id])
  useCrumb(view ? jobLabel(view.job) : undefined)
  if (err) return <Empty title="This job is not there" icon="alert">{err} · <Link to="/jobs">All jobs</Link></Empty>
  if (!view) return <Loading />
  return <Job1 view={view} reload={load} onDeleted={() => { toast('Deleted'); navigate('/board') }} />
}

type Sheet = '' | 'edit' | 'step' | 'log' | 'close' | 'hired' | 'delete' | 'interview' | 'offer'

function Job1({ view, reload, onDeleted }: { view: JobView; reload: () => void; onDeleted: () => void }) {
  const j = view.job
  const { stages } = useData()
  const appState = useAppState()
  const { toast } = useToast()
  const [sheet, setSheet] = useState<Sheet>('')
  const [make, setMake] = useState<{ target: string; kind?: string; interview?: number } | null>(null)
  const [editIV, setEditIV] = useState<Interview | null>(null)
  const [prepIV, setPrepIV] = useState<Interview | null>(null)
  const [moving, setMoving] = useState(0)
  const open = (stages || []).filter(s => OPEN_KINDS.includes(s.kind))
  const cur = (stages || []).find(s => s.id === j.stage_id)
  const move = async (stageID: number) => {
    if (stageID === j.stage_id) return
    setMoving(stageID)
    try { await api.post(`${V1}/jobs/${j.id}/move`, { stage_id: stageID }); announceChange() } catch (e) { toast(errMsg(e), 'err') } finally { setMoving(0) }
  }
  const star = async () => { await api.put(`${V1}/jobs/${j.id}`, { starred: !j.starred }).catch(() => {}); reload() }
  const del = async () => { try { await api.del(`${V1}/jobs/${j.id}`); announceChange(); onDeleted() } catch (e) { toast(errMsg(e), 'err') } }
  const linkContact = async () => {
    try { const m = await api.post<Made>(`${V1}/jobs/${j.id}/make`, { target: 'contact' }); toast(`Linked: ${m.title}`); announceChange() } catch (e) { toast(errMsg(e), 'err') }
  }
  const applied = (stages || []).find(s => s.kind === 'applied')
  const more: MenuItem[] = [
    { label: 'Change the job', icon: 'edit', onSelect: () => setSheet('edit') },
    { label: 'Write something down', icon: 'note', onSelect: () => setSheet('log') },
    { label: 'Plan an interview', icon: 'video', onSelect: () => setSheet('interview') },
    { label: view.offer ? 'The offer' : 'Add an offer', icon: 'gift', onSelect: () => setSheet('offer') },
    'sep',
    ...(j.status === 'open' ? [{ label: 'Hired', icon: 'trophy', onSelect: () => setSheet('hired') }, { label: 'Close it', icon: 'x-circle', onSelect: () => setSheet('close') }] : open.length ? [{ label: 'Open it again', icon: 'undo', onSelect: () => move(open[Math.min(1, open.length - 1)].id) }] : []),
    'sep',
    { label: 'Delete', icon: 'trash', danger: true, onSelect: () => setSheet('delete') },
  ]
  const contacts = appState('contacts', 'contacts:read')
  const contactURL = (ref: string) => contacts.app?.open_url && ref ? `${contacts.app.open_url.replace(/\/$/, '')}/contacts/${ref}` : ''
  const t = todayISO()
  const salary = salaryText(j)
  return (
    <div className="job-page">
      <header className="job-head">
        <div className="grow min0">
          <div className="job-kicker">
            <span className="stage-pill" style={{ ['--st' as string]: stageColor(cur) }}>{j.stage_name}</span>
            {j.status === 'open' && <span className="muted small">{j.applied_on ? `Applied ${dayWords(j.applied_on, t)}` : 'Not applied yet'}{j.days_in_stage ? ` · ${j.days_in_stage} ${j.days_in_stage === 1 ? 'day' : 'days'} in this stage` : ''}</span>}
            {j.status !== 'open' && j.closed_at && <span className="muted small">{j.status === 'hired' ? 'Hired' : endLabel(j.end_reason)} · {fmtDate(j.closed_at.slice(0, 10))}</span>}
          </div>
          <h2 className="job-title">{j.title || j.company}</h2>
          <div className="job-sub">
            {j.company && j.title && <span className="job-co"><Icon name="building" size={14} />{j.company}</span>}
            {(j.location || j.work_mode) && <span><Icon name="pin" size={14} />{[j.location, modeLabel(j.work_mode)].filter(Boolean).join(' · ')}</span>}
            {salary && <span className="tnum"><Icon name="wallet" size={14} />{salary}</span>}
            {j.url && <a href={j.url} target="_blank" rel="noreferrer"><Icon name="external" size={14} />Posting</a>}
          </div>
        </div>
        <div className="row job-tools">
          <button type="button" className={`btn ghost icon-only ${j.starred ? 'star-on' : ''}`} onClick={star} aria-label={j.starred ? 'Unstar' : 'Star'} title={j.starred ? 'Starred' : 'Star'}><Icon name="star" size={18} /></button>
          <button type="button" className="btn" onClick={() => setSheet('edit')}><Icon name="edit" size={15} /><span className="hide-sm">Change</span></button>
          {j.status === 'open' && cur?.kind === 'saved' && applied && <button type="button" className="btn primary" onClick={() => move(applied.id)} disabled={!!moving}>{moving ? <Spinner /> : <Icon name="send" size={15} />}I applied</button>}
          {j.status === 'open' && cur?.kind !== 'saved' && <button type="button" className="btn hide-sm" onClick={() => setSheet('interview')}><Icon name="video" size={15} />Interview</button>}
          <MenuButton label="More" items={more} align="right" width={230}><Icon name="more" size={18} /></MenuButton>
        </div>
      </header>

      {j.status === 'open' && open.length > 0 && <nav className="stepper" aria-label="Stage">
        {open.map(s => {
          const i = open.findIndex(x => x.id === j.stage_id)
          const k = open.findIndex(x => x.id === s.id)
          return (
            <button key={s.id} type="button" className={`stepper-step ${s.id === j.stage_id ? 'cur' : k < i ? 'past' : ''}`} style={{ ['--st' as string]: stageColor(s) }} onClick={() => move(s.id)} disabled={!!moving} title={`Move to ${s.name}`}>
              {moving === s.id ? <Spinner /> : null}<span className="ellipsis">{s.name}</span>
            </button>
          )
        })}
      </nav>}

      <div className="job-grid">
        <div className="job-main stack loose">
          {j.status === 'open' && <section className={`next-card ${j.overdue ? 'overdue' : ''} ${!j.next_action ? 'none' : ''}`}>
            <div className="next-ico"><Icon name={j.next_action ? 'target' : j.stale ? 'clock' : 'alert'} size={18} /></div>
            <div className="grow">
              <div className="next-l">Next step{j.next_due && <> · <span className={j.overdue ? 'danger-text' : ''}>{j.overdue ? 'was due ' : ''}{fmtDate(j.next_due)}</span></>}</div>
              <div className="next-t">{j.next_action || (j.stale ? 'No answer for a while. Follow up, or close it.' : cur?.kind === 'saved' ? 'Apply, or plan when you will.' : 'No next step yet.')}</div>
              {j.next_task_ref && <div className="muted small"><Icon name="check-square" size={12} className="inline-ico" /> Also in Pocket Tasks</div>}
            </div>
            {j.stale && !j.next_action
              ? <button type="button" className="btn primary" onClick={() => setMake({ target: 'email', kind: 'follow_up' })}><Icon name="mail" size={15} />Follow up</button>
              : <button type="button" className="btn primary" onClick={() => setSheet('step')}>{j.next_action ? <><Icon name="check" size={15} />Done</> : <><Icon name="plus" size={15} />Set it</>}</button>}
          </section>}
          {j.status !== 'open' && j.outcome_note && <section className="callout"><b>{j.status === 'hired' ? 'Agreed' : 'Note'}:</b> {j.outcome_note}</section>}

          <Interviews job={j} list={view.interviews} onAdd={() => setSheet('interview')} onEdit={setEditIV} onPrep={setPrepIV} onThanks={iv => setMake({ target: 'email', kind: 'thank_you', interview: iv.id })} />
          {view.offer && <OfferCard offer={view.offer} onEdit={() => setSheet('offer')} />}
          <AIHelp job={j} onSaved={reload} />
          <Posting job={j} onEdit={() => setSheet('edit')} />
          <Timeline job={j} acts={view.activities} onLog={() => setSheet('log')} />
          <Elsewhere job={j} />
        </div>

        <aside className="job-side stack">
          <section className="card side-card">
            <h3 className="side-h">Who to talk to</h3>
            {!(j.contact_name || j.contact_email) ? <p className="muted small">Nobody yet. <button type="button" className="link-btn" onClick={() => setSheet('edit')}>Add the recruiter</button></p> : <>
              {j.contact_name && <div className="who-row"><Icon name="user" size={15} /><span className="grow ellipsis">{contactURL(j.contact_ref) ? <a href={contactURL(j.contact_ref)} target="_blank" rel="noreferrer">{j.contact_name}</a> : j.contact_name}</span></div>}
              {j.contact_email && <div className="who-row"><Icon name="mail" size={15} /><a className="grow ellipsis" href={`mailto:${j.contact_email}`}>{j.contact_email}</a></div>}
              {j.contact_phone && <div className="who-row"><Icon name="phone" size={15} /><a className="grow ellipsis" href={`tel:${j.contact_phone.replace(/\s/g, '')}`}>{j.contact_phone}</a></div>}
            </>}
            {j.company && <div className="who-row"><Icon name="building" size={15} /><span className="grow ellipsis">{contactURL(j.company_ref) ? <a href={contactURL(j.company_ref)} target="_blank" rel="noreferrer">{j.company}</a> : <Link to={`/companies?q=${encodeURIComponent(j.company)}`}>{j.company}</Link>}</span></div>}
            {(j.contact_name || j.contact_email || j.company) && !(j.contact_ref || j.company_ref) && (contacts.ok
              ? <button type="button" className="btn sm mt" onClick={linkContact}><Icon name="users" size={14} />Find or add in Pocket Contacts</button>
              : <p className="muted small mt" title={offTitle(contacts.app, 'contacts')}><Icon name="info" size={12} className="inline-ico" /> Kept here{contacts.app?.state === 'missing' ? '' : '; connect Pocket Contacts to link their card'}.</p>)}
          </section>

          <section className="card side-card">
            <h3 className="side-h">Do it in your other apps</h3>
            <MadeList made={view.made} onChange={reload} />
            <div className="make-grid">
              {[{ target: 'email', label: 'Email' }, { target: 'task', label: 'Task' }, { target: 'note', label: 'Note' }].map(m => {
                const slug = TARGET_APP[m.target]
                const st = appState(slug, MAKE_SCOPE[m.target])
                const off = !st.ok && m.target !== 'email'
                return (
                  <button key={m.target} type="button" className={`make-btn ${!st.ok ? 'off' : ''}`} onClick={() => setMake({ target: m.target })} disabled={off || st.loading} title={st.ok ? APP_INFO[slug].name : offTitle(st.app, slug) + (m.target === 'email' ? ' You can still write it and send it from your own email app.' : '')}>
                    <Icon name={APP_INFO[slug].icon} size={15} />{m.label}
                  </button>
                )
              })}
            </div>
          </section>

          <section className="card side-card">
            <h3 className="side-h">About</h3>
            <dl className="facts">
              <dt>Want it</dt><dd><Stars value={j.excitement} size={14} /></dd>
              {j.fit_score >= 0 && <><dt>Fit</dt><dd><span className={`fit fit-${fitTone(j.fit_score)}`}>{j.fit_score}%</span></dd></>}
              <dt>CV</dt><dd>{j.cv_name ? <Link to="/cvs">{j.cv_name}</Link> : <span className="muted">Your main CV</span>}</dd>
              {j.employment && <><dt>Kind</dt><dd>{EMPLOYMENTS.find(e => e.value === j.employment)?.label}</dd></>}
              {j.apply_by && <><dt>Apply by</dt><dd className={j.status === 'open' && !j.applied_on && j.apply_by < t ? 'danger-text' : ''}>{fmtDate(j.apply_by)}</dd></>}
              <dt>Found on</dt><dd>{sourceLabel(j.source)}{j.source_app && ` (${APP_INFO[j.source_app as keyof typeof APP_INFO]?.name || 'Pocket ' + j.source_app})`}</dd>
              {j.tags.length > 0 && <><dt>Tags</dt><dd className="chips">{j.tags.map(x => <Link key={x} className="chip" to={`/jobs?tag=${encodeURIComponent(x)}`}>{x}</Link>)}</dd></>}
              <dt>Saved</dt><dd>{fmtDate(j.created_at.slice(0, 10))}</dd>
              <dt>Last touched</dt><dd>{ago(j.last_activity_at)}</dd>
            </dl>
            {j.notes && <p className="pre small mt">{j.notes}</p>}
          </section>
        </aside>
      </div>

      {sheet === 'edit' && <JobForm job={j} onClose={() => setSheet('')} onSaved={() => reload()} />}
      {sheet === 'step' && <StepSheet job={j} onClose={() => setSheet('')} onDone={() => { setSheet(''); reload() }} />}
      {sheet === 'log' && <LogSheet job={j} onClose={() => setSheet('')} onDone={() => { setSheet(''); reload() }} />}
      {sheet === 'close' && <CloseSheet job={j} onClose={() => setSheet('')} onDone={() => { setSheet(''); reload() }} />}
      {sheet === 'hired' && <HiredSheet job={j} onClose={() => setSheet('')} onDone={() => { setSheet(''); reload() }} />}
      {sheet === 'interview' && <InterviewSheet job={j} onClose={() => setSheet('')} onDone={() => { setSheet(''); reload() }} />}
      {sheet === 'offer' && <OfferSheet job={j} offer={view.offer} onClose={() => setSheet('')} onDone={() => { setSheet(''); reload() }} />}
      {editIV && <InterviewSheet job={j} interview={editIV} onClose={() => setEditIV(null)} onDone={() => { setEditIV(null); reload() }} />}
      {prepIV && <QuestionsSheet job={j} interview={prepIV} onClose={() => setPrepIV(null)} onDone={() => { setPrepIV(null); reload() }} />}
      {sheet === 'delete' && <Confirm title="Delete this job?" message={<>“{jobLabel(j)}” with its timeline, interviews and offer. What it has in your other apps stays there.</>} confirmLabel="Delete" onConfirm={del} onCancel={() => setSheet('')} />}
      {make && <MakeSheet job={j} target={make.target} emailKind={make.kind} interviewID={make.interview} onClose={() => setMake(null)} onDone={() => { setMake(null); reload() }} />}
    </div>
  )
}

export const fitTone = (n: number) => n >= 75 ? 'good' : n >= 55 ? 'ok' : 'low'

// ---- interviews ----

function Interviews({ job, list, onAdd, onEdit, onPrep, onThanks }: { job: Job; list: Interview[]; onAdd: () => void; onEdit: (iv: Interview) => void; onPrep: (iv: Interview) => void; onThanks: (iv: Interview) => void }) {
  if (!list.length && job.status !== 'open') return null
  const upcoming = list.filter(iv => !iv.past)
  const past = list.filter(iv => iv.past).reverse()
  return (
    <section>
      <div className="sec-head"><h3>Interviews</h3><span className="spacer" /><button type="button" className="btn sm" onClick={onAdd}><Icon name="plus" size={14} />Plan one</button></div>
      {!list.length ? <p className="muted small">None yet. When they invite you, plan it here: it goes in your calendar, and Pocket AI helps you prepare.</p> : (
        <div className="stack">
          {upcoming.map(iv => <InterviewCard key={iv.id} iv={iv} job={job} onEdit={() => onEdit(iv)} onPrep={() => onPrep(iv)} onThanks={() => onThanks(iv)} />)}
          {past.map(iv => <InterviewCard key={iv.id} iv={iv} job={job} onEdit={() => onEdit(iv)} onPrep={() => onPrep(iv)} onThanks={() => onThanks(iv)} />)}
        </div>
      )}
    </section>
  )
}

function InterviewCard({ iv, job, onEdit, onPrep, onThanks }: { iv: Interview; job: Job; onEdit: () => void; onPrep: () => void; onThanks: () => void }) {
  const k = interviewKind(iv.kind)
  const appState = useAppState()
  const { toast } = useToast()
  const cal = appState('calendar', 'calendar:write')
  const [busy, setBusy] = useState(false)
  const toCal = async () => {
    setBusy(true)
    try { await api.post(`${V1}/interviews/${iv.id}/calendar`); toast('In your calendar'); announceChange() } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  const soon = !iv.past && iv.starts_at.slice(0, 10) <= new Date(Date.now() + 10 * 86400000).toISOString().slice(0, 10)
  return (
    <article className={`iv-card ${iv.past ? 'past' : ''}`}>
      <div className="iv-ico"><Icon name={k.icon} size={17} /></div>
      <div className="grow min0">
        <div className="iv-h"><b>{k.label}</b><span className="muted">· {whenText(iv.starts_at)}{iv.minutes && iv.starts_at.length > 10 ? ` · ${iv.minutes} min` : ''}</span>
          {iv.outcome && <span className={`pill out-${iv.outcome}`}>{iv.outcome === 'good' ? 'Went well' : iv.outcome === 'okay' ? 'Okay' : 'Not great'}</span>}</div>
        {(iv.with_people || iv.location) && <div className="muted small ellipsis">{[iv.with_people && `With ${iv.with_people}`, iv.location].filter(Boolean).map((x, i) => <span key={i}>{i ? ' · ' : ''}{/^https?:/.test(x!) ? <a href={x!} target="_blank" rel="noreferrer">Join the call</a> : x}</span>)}</div>}
        {iv.feedback && <p className="small pre iv-note">{iv.feedback}</p>}
        {!iv.past && soon && <IvExtras iv={iv} />}
        <div className="iv-tools">
          <button type="button" className="btn sm" onClick={onPrep}><Icon name="brain" size={14} />{iv.questions.length ? `${iv.questions.length} questions` : 'Prepare'}</button>
          {iv.past && !iv.thanks_sent && job.status === 'open' && <button type="button" className="btn sm" onClick={onThanks}><Icon name="mail" size={14} />Thank them</button>}
          {!iv.past && (iv.event_url
            ? <a className="btn sm ghost" href={iv.event_url} target="_blank" rel="noreferrer"><Icon name="calendar-check" size={14} />In your calendar</a>
            : cal.ok ? <button type="button" className="btn sm ghost" onClick={toCal} disabled={busy}>{busy ? <Spinner /> : <Icon name="calendar-plus" size={14} />}To Pocket Calendar</button>
              : <a className="btn sm ghost" href={`${V1}/interviews/${iv.id}/interview.ics`} download title={offTitle(cal.app, 'calendar')}><Icon name="download" size={14} />For your calendar</a>)}
          <button type="button" className="btn sm ghost" onClick={onEdit}><Icon name="edit" size={14} />{iv.past && !iv.outcome ? 'How it went' : 'Change'}</button>
        </div>
      </div>
    </article>
  )
}

/** For an interview in the next days: the weather there and what else is in the calendar then. */
function IvExtras({ iv }: { iv: Interview }) {
  const [wx, setWx] = useState<{ state: string; place?: string; hour?: Record<string, unknown>; day?: Record<string, unknown> } | null>(null)
  const [clash, setClash] = useState<{ state: string; items: { title: string; start: string; url: string }[] } | null>(null)
  useEffect(() => {
    let alive = true
    api.get<typeof wx>(`${V1}/interviews/${iv.id}/weather`).then(r => { if (alive) setWx(r) }).catch(() => {})
    api.get<typeof clash>(`${V1}/interviews/${iv.id}/clashes`).then(r => { if (alive) setClash(r) }).catch(() => {})
    return () => { alive = false }
  }, [iv.id, iv.starts_at, iv.location])
  const h = wx?.hour as { temp?: number; condition?: string; precip_prob?: number } | undefined
  const d = wx?.day as { t_max?: number; t_min?: number; text?: string; condition?: string; precip_prob?: number } | undefined
  return <>
    {wx?.state === 'ok' && <div className="iv-extra"><Icon name="cloud" size={13} />{wx.place}: {h ? `${Math.round(h.temp ?? 0)}°, ${h.condition || ''}${h.precip_prob ? ` · ${Math.round(h.precip_prob)}% rain` : ''}` : d ? `${Math.round(d.t_min ?? 0)}–${Math.round(d.t_max ?? 0)}°, ${d.text || d.condition || ''}` : ''}</div>}
    {clash?.state === 'ok' && clash.items.length > 0 && <div className="iv-extra warn"><Icon name="alert" size={13} />Clashes with {clash.items.slice(0, 2).map((c, i) => <span key={i}>{i ? ', ' : ''}{c.url ? <a href={c.url} target="_blank" rel="noreferrer">{c.title}</a> : c.title}</span>)}</div>}
  </>
}

// ---- the offer ----

function OfferCard({ offer, onEdit }: { offer: Offer; onEdit: () => void }) {
  const per = { year: 'a year', month: 'a month', hour: 'an hour' }[offer.period]
  return (
    <section className="card offer-card">
      <div className="row"><Icon name="gift" size={16} /><h3 className="grow">The offer</h3><span className={`pill offer-${offer.status}`}>{{ open: 'To decide', negotiating: 'Negotiating', accepted: 'Accepted', declined: 'Declined' }[offer.status]}</span><button type="button" className="btn sm" onClick={onEdit}><Icon name="edit" size={14} />Change</button></div>
      <div className="offer-pay tnum">{offer.base ? <>{fmtMoney(offer.base, offer.currency)} <span className="muted small">{per}</span></> : <span className="muted">No pay written yet</span>}</div>
      <div className="offer-facts muted small">
        {offer.period !== 'year' && offer.base > 0 && <span>≈ {fmtMoney(offer.yearly, offer.currency, true)} a year</span>}
        {offer.bonus > 0 && <span>+ {fmtMoney(offer.bonus, offer.currency, true)} bonus</span>}
        {offer.pto_days > 0 && <span>{offer.pto_days} days off</span>}
        {offer.equity && <span>{offer.equity}</span>}
        {offer.start_date && <span>Start {fmtDate(offer.start_date)}</span>}
        {offer.decide_by && <span className={offer.status === 'open' && offer.decide_by <= todayISO() ? 'danger-text' : ''}>Decide by {fmtDate(offer.decide_by)}</span>}
      </div>
      {offer.benefits && <p className="small pre">{offer.benefits}</p>}
      <Link to="/offers" className="small">Compare with your other offers</Link>
    </section>
  )
}

// ---- Pocket AI ----

type AIOut = { kind: 'fit'; score: number; verdict: string; strengths: string[]; gaps: string[]; advice: string; cv: string } | { kind: 'letter' | 'tailor' | 'research' | 'summary'; text: string; cv?: string }

function AIHelp({ job, onSaved }: { job: Job; onSaved: () => void }) {
  const ai = useAI()
  const { settings } = useApp()
  const { toast } = useToast()
  const [busy, setBusy] = useState('')
  const [out, setOut] = useState<AIOut | null>(null)
  const run = async (want: 'fit' | 'letter' | 'tailor' | 'research' | 'summary') => {
    setBusy(want)
    try {
      const r = await api.post<Record<string, unknown>>(`${V1}/jobs/${job.id}/ai`, { want })
      setOut({ kind: want, ...r } as AIOut)
      if (want === 'fit') onSaved()
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy('') }
  }
  const keepLetter = async (text: string) => {
    try { await api.put(`${V1}/jobs/${job.id}`, { cover_letter: text }); toast('Kept with the job'); onSaved() } catch (e) { toast(errMsg(e), 'err') }
  }
  if (!settings?.ai_help) return null
  const tools: { want: 'fit' | 'letter' | 'tailor' | 'research' | 'summary'; icon: string; label: string; help: string }[] = [
    { want: 'fit', icon: 'target', label: 'How well do I fit?', help: 'Your CV against what they ask' },
    { want: 'letter', icon: 'file-text', label: 'Cover letter', help: 'Written for this job' },
    { want: 'tailor', icon: 'edit', label: 'Change my CV for it', help: 'What to stress and reword' },
    { want: 'research', icon: 'search', label: 'What to find out', help: 'Questions and red flags' },
  ]
  return (
    <section>
      <div className="sec-head"><h3>Pocket AI</h3>{!ai && <span className="muted small">· not set up (<Link to="/settings/ai">Settings → AI</Link>)</span>}</div>
      {job.fit_score >= 0 && job.fit_note && <div className={`fit-box fit-${fitTone(job.fit_score)}`}><span className="fit-num">{job.fit_score}%</span><p className="small pre grow">{job.fit_note}</p></div>}
      <div className="ai-tools">
        {tools.map(x => (
          <button key={x.want} type="button" className="ai-tool" onClick={() => run(x.want)} disabled={!ai || !!busy}>
            <span className="ai-tool-ico">{busy === x.want ? <Spinner /> : <Icon name={x.icon} size={16} />}</span>
            <span className="grow min0"><b>{x.label}</b><span className="muted small ellipsis block-line">{x.help}</span></span>
          </button>
        ))}
      </div>
      {job.cover_letter && !out && <details className="kept"><summary><Icon name="file-text" size={14} />Your cover letter for this job</summary><p className="pre small">{job.cover_letter}</p><button type="button" className="btn sm" onClick={() => copyText(job.cover_letter).then(ok => ok && toast('Copied'))}><Icon name="copy" size={14} />Copy</button></details>}
      {out && <Modal title={out.kind === 'fit' ? 'How well you fit' : out.kind === 'letter' ? 'Cover letter' : out.kind === 'tailor' ? 'Your CV for this job' : 'What to find out'} onClose={() => setOut(null)} size="lg" footer={<>
        {out.kind !== 'fit' && <button type="button" className="btn" onClick={() => copyText(out.text).then(ok => ok && toast('Copied'))}><Icon name="copy" size={15} />Copy</button>}
        {out.kind === 'letter' && <button type="button" className="btn" onClick={() => keepLetter(out.text)}><Icon name="check" size={15} />Keep it with the job</button>}
        <button type="button" className="btn primary" onClick={() => setOut(null)}>Close</button>
      </>}>
        {out.kind === 'fit' ? <div className="stack">
          <div className={`fit-box fit-${fitTone(out.score)}`}><span className="fit-num">{out.score}%</span><p className="grow">{out.verdict}</p></div>
          {out.strengths?.length > 0 && <div><h4 className="form-sec-h ok-text">What matches</h4><ul className="plain-list">{out.strengths.map(s => <li key={s}><Icon name="check" size={13} />{s}</li>)}</ul></div>}
          {out.gaps?.length > 0 && <div><h4 className="form-sec-h warn-text">What they ask that your CV does not show</h4><ul className="plain-list">{out.gaps.map(s => <li key={s}><Icon name="alert" size={13} />{s}</li>)}</ul></div>}
          {out.advice && <p className="callout">{out.advice}</p>}
          {out.cv && <p className="muted small">Compared with “{out.cv}”.</p>}
        </div> : <div className="stack"><div className="ai-text pre">{out.text}</div>{out.cv && <p className="muted small">From “{out.cv}”.</p>}</div>}
      </Modal>}
    </section>
  )
}

// ---- the posting ----

function Posting({ job, onEdit }: { job: Job; onEdit: () => void }) {
  const [openP, setOpenP] = useState(false)
  if (!job.posting) return <section><div className="sec-head"><h3>The posting</h3></div><p className="muted small">Not kept yet. <button type="button" className="link-btn" onClick={onEdit}>Paste its text</button> — then Pocket AI can check the fit and prepare you{job.url ? <>, or open <a href={job.url} target="_blank" rel="noreferrer">the posting</a></> : ''}.</p></section>
  const long = job.posting.length > 700
  return (
    <section>
      <div className="sec-head"><h3>The posting</h3><span className="spacer" />{job.url && <a className="btn sm ghost" href={job.url} target="_blank" rel="noreferrer"><Icon name="external" size={14} />Open</a>}</div>
      <div className={`posting ${openP || !long ? 'open' : ''}`}><p className="pre small">{openP || !long ? job.posting : job.posting.slice(0, 700) + '…'}</p></div>
      {long && <button type="button" className="link-btn" onClick={() => setOpenP(!openP)}>{openP ? 'Show less' : 'Show all of it'}</button>}
    </section>
  )
}

// ---- linked things, the timeline, the other apps ----

function MadeList({ made, onChange }: { made: Made[]; onChange: () => void }) {
  const { toast } = useToast()
  if (!made.length) return <p className="muted small">Nothing yet. Make it from here: it is linked back to the job.</p>
  const forget = async (m: Made) => { try { await api.del(`${V1}/made/${m.id}`); onChange() } catch (e) { toast(errMsg(e), 'err') } }
  return (
    <ul className="made-list">
      {made.map(m => {
        const slug = TARGET_APP[m.target] || (m.app as keyof typeof APP_INFO)
        const info = APP_INFO[slug as keyof typeof APP_INFO]
        return (
          <li key={m.id} className={m.error ? 'bad' : ''}>
            <Icon name={m.error ? 'alert' : info?.icon || 'link'} size={14} />
            <div className="grow min0">
              <div className="ellipsis">{m.url ? <a href={m.url} target="_blank" rel="noreferrer">{m.title || TARGET_LABEL[m.target]}</a> : (m.title || TARGET_LABEL[m.target])}</div>
              <div className="muted tiny">{m.error || <>{TARGET_LABEL[m.target] || m.target}{m.state && <> · <span className={`state st-${m.state}`}>{m.state}</span></>}</>}</div>
            </div>
            <button type="button" className="btn ghost icon-only sm" onClick={() => forget(m)} aria-label="Forget" title="Forget here (it stays in the other app)"><Icon name="x" size={13} /></button>
          </li>
        )
      })}
    </ul>
  )
}

function Timeline({ job, acts, onLog }: { job: Job; acts: Activity[]; onLog: () => void }) {
  const { toast } = useToast()
  const [confirm, setConfirm] = useState<Activity | null>(null)
  const del = async (a: Activity) => { try { await api.del(`${V1}/activities/${a.id}`); announceChange() } catch (e) { toast(errMsg(e), 'err') } setConfirm(null) }
  return (
    <section>
      <div className="sec-head"><h3>Timeline</h3><span className="spacer" /><button type="button" className="btn sm" onClick={onLog}><Icon name="plus" size={14} />Write it down</button></div>
      {acts.length === 0 ? <p className="muted small">Nothing yet.</p> : (
        <ol className="timeline">
          {acts.map(a => {
            const info = ACTIVITY[a.kind] || ACTIVITY.note
            const own = ['note', 'call', 'email', 'step'].includes(a.kind) && !a.app
            return (
              <li key={a.id} className={`tl tl-${a.kind}`}>
                <span className="tl-dot"><Icon name={info.icon} size={13} /></span>
                <div className="tl-body">
                  <div className="tl-h">
                    <span className="tl-t">{a.url ? <a href={a.url} target="_blank" rel="noreferrer">{a.title || info.label}</a> : (a.title || info.label)}</span>
                    <span className="muted tiny tl-when" title={fmtDateTime(a.at)}>{fmtDateTime(a.at, new Date(a.at).getFullYear() !== new Date().getFullYear())}</span>
                    {own && <button type="button" className="btn ghost icon-only sm tl-x" onClick={() => setConfirm(a)} aria-label="Delete this line"><Icon name="trash" size={12} /></button>}
                  </div>
                  {a.body && <p className="tl-text pre">{a.body}</p>}
                  {a.source_app && <span className="muted tiny">from {APP_INFO[a.source_app as keyof typeof APP_INFO]?.name || 'Pocket ' + a.source_app}</span>}
                </div>
              </li>
            )
          })}
        </ol>
      )}
      {confirm && <Confirm title="Delete this line?" message={confirm.title || confirm.body.slice(0, 80)} confirmLabel="Delete" onConfirm={() => del(confirm)} onCancel={() => setConfirm(null)} />}
      {job.activities > acts.length && <p className="muted small">Older lines are in the export.</p>}
    </section>
  )
}

/** What the other apps know about the company, read live, each on its own. */
function Elsewhere({ job }: { job: Job }) {
  const appState = useAppState()
  const [secs, setSecs] = useState<ElseSection[] | null>(null)
  useEffect(() => {
    let alive = true
    setSecs(null)
    api.get<ElseSection[]>(`${V1}/jobs/${job.id}/elsewhere`).then(s => { if (alive) setSecs(s) }).catch(() => { if (alive) setSecs([]) })
    return () => { alive = false }
  }, [job.id, job.contact_email, job.company, job.contact_name])
  if (!secs) return <section><div className="sec-head"><h3>From your other apps</h3></div><div className="skeleton" style={{ height: 64 }} /></section>
  const shown = secs.map(s => ({ ...s, items: s.items || [] })).filter(s => s.state !== 'none' || s.items.length)
  const off = shown.filter(s => s.state === 'off')
  return (
    <section>
      <div className="sec-head"><h3>From your other apps</h3></div>
      <div className="stack">
        {off.length === 1 && <AppOff slug={off[0].app} app={appState(off[0].app).app} compact />}
        {off.length > 1 && <div className="app-off compact else-off">
          <span className="app-off-ico"><Icon name="link" size={15} /></span>
          <div className="grow">
            <div className="app-off-h">Not connected yet</div>
            <div className="muted small">{off.map((s, i) => <span key={s.app} title={offTitle(appState(s.app).app, s.app)}>{i ? ', ' : ''}{APP_INFO[s.app].name}</span>)} — with them, your emails with the company and what is in your calendar about it show here. <Link to="/settings/apps">Connected apps</Link></div>
          </div>
        </div>}
        {shown.map(s => {
          if (s.state === 'off' || s.state === 'none') return null
          if (s.state === 'error') return <div key={s.app} className="app-off compact"><span className="app-off-ico"><Icon name={APP_INFO[s.app].icon} size={15} /></span><div className="grow muted small">{s.detail}</div></div>
          return (
            <div key={s.app} className="else-sec">
              <div className="else-h"><Icon name={APP_INFO[s.app].icon} size={14} />{s.name}<span className="muted small">· {s.items.length ? s.items.length : 'nothing'} {s.app === 'mail' ? 'about it' : 'in your calendar'}</span></div>
              {s.items.slice(0, 6).map((it, i) => (
                <a key={i} className="else-row" href={it.url || undefined} target="_blank" rel="noreferrer">
                  {it.kind === 'email' && <Icon name={it.out ? 'send' : 'inbox'} size={13} />}
                  {it.kind === 'meeting' && <Icon name="calendar" size={13} />}
                  <span className="grow min0"><span className="ellipsis block-line">{it.title}</span>{it.body && <span className="muted tiny ellipsis block-line">{it.body}</span>}</span>
                  <span className="muted tiny">{it.at ? fmtDate(it.at.slice(0, 10), false) : ''}</span>
                </a>
              ))}
            </div>
          )
        })}
        {shown.every(s => s.state === 'none') && <p className="muted small">Add the company or the recruiter’s email to see your emails and calendar about it here.</p>}
      </div>
    </section>
  )
}
