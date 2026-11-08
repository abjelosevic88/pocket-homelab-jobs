// Today: the week's goal, the interviews coming, what to do now, the
// applications to follow up, the jobs to apply for and the offers to decide.
import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { dayWords, interviewKind, jobLabel, whenText } from '../lib/jobs'
import { useOnChange } from '../lib/data'
import { fmtDate, fmtMoney, todayISO, weekday } from '../lib/format'
import type { Job, TodayView } from '../lib/types'
import { MakeSheet, StepSheet } from '../components/JobActions'
import { Ring } from '../components/Chart'
import { Icon } from '../components/icons'
import { openCreate } from '../components/Layout'
import { Empty, Loading } from '../components/ui'

export default function Today() {
  const { user } = useApp()
  const [v, setV] = useState<TodayView | null>(null)
  const [step, setStep] = useState<Job | null>(null)
  const [follow, setFollow] = useState<Job | null>(null)
  const load = useCallback(async () => { try { setV(await api.get<TodayView>(`${V1}/today`)) } catch { setV(x => x) } }, [])
  useEffect(() => { load() }, [load])
  useOnChange(load)
  if (!v) return <Loading />
  const s = v.summary
  const L = v.lists
  const t = todayISO()
  const hour = new Date().getHours()
  const hello = hour < 12 ? 'Good morning' : hour < 18 ? 'Good afternoon' : 'Good evening'
  const nothing = s.open === 0 && s.hired + s.closed === 0
  const todo = L.overdue.length + L.today.length
  const ivToday = v.interviews.filter(iv => iv.starts_at.slice(0, 10) === t).length
  const goalPct = s.weekly_goal ? Math.min(100, Math.round((s.applied_week / s.weekly_goal) * 100)) : 0
  const maxDay = Math.max(1, ...v.week.map(d => d.applied))
  return (
    <div className="page stack loose">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">{hello}{user?.name ? `, ${user.name.split(' ')[0]}` : ''}</h1>
          <p className="muted">{[ivToday && `${ivToday} ${ivToday === 1 ? 'interview' : 'interviews'} today`, todo && `${todo} ${todo === 1 ? 'thing' : 'things'} to do`].filter(Boolean).join(' · ') || (s.open ? 'Nothing due today.' : 'No jobs yet.')}</p></div>
        <button type="button" className="btn primary hide-sm" onClick={() => openCreate('job')}><Icon name="plus" size={15} />Add a job</button>
      </div>

      {nothing ? <Empty title="Start your search" icon="briefcase">
        Save the jobs you want — paste a link and Pocket Jobs reads the posting — then follow each one from applied to hired. <button type="button" className="link-btn" onClick={() => openCreate('job')}>Add a job</button>, or <button type="button" className="link-btn" onClick={() => openCreate('import')}>bring them from Huntr, Teal or a spreadsheet</button>. Add <Link to="/cvs">your CV</Link> too: Pocket AI uses it.
      </Empty> : <div className="today-top">
        <section className="card goal-card">
          {s.weekly_goal > 0 ? <Ring pct={goalPct} size={74} stroke={8} color={goalPct >= 100 ? 'var(--success)' : 'var(--accent)'}><span className="ring-num"><b>{s.applied_week}</b><small>of {s.weekly_goal}</small></span></Ring>
            : <span className="big-num">{s.applied_week}</span>}
          <div className="grow min0">
            <div className="goal-h">Applied this week</div>
            <div className="week-days" aria-label="Applications each day this week">
              {v.week.map(d => <span key={d.date} className={`wd ${d.date === t ? 'today' : ''} ${d.date > t ? 'future' : ''}`} title={`${fmtDate(d.date)}: ${d.applied}`}>
                <span className="wd-bar"><span style={{ height: `${Math.round((d.applied / maxDay) * 100)}%` }} /></span><span className="wd-l">{weekday(d.date).slice(0, 2)}</span></span>)}
            </div>
            <div className="muted small">{s.weekly_goal ? (s.applied_week >= s.weekly_goal ? 'Goal reached — well done.' : `${s.weekly_goal - s.applied_week} to go`) : <Link to="/settings/general">Set a weekly goal</Link>}</div>
          </div>
        </section>
        <div className="tiles">
          <Link to="/jobs?kind=applied" className="tile"><div className="tile-head"><span className="tile-ico"><Icon name="send" size={14} /></span>Waiting to hear</div><div className="tile-val">{s.waiting}</div><div className="muted small">{s.follow_up ? `${s.follow_up} to follow up` : 'applications out'}</div></Link>
          <Link to="/interviews" className="tile" style={{ ['--tile' as string]: '#7048e8' }}><div className="tile-head"><span className="tile-ico"><Icon name="video" size={14} /></span>Interviews</div><div className="tile-val">{s.interviews_upcoming}</div><div className="muted small ellipsis">{s.next_interview ? `next ${whenText(s.next_interview.starts_at)}` : 'none coming'}</div></Link>
          <Link to="/offers" className="tile" style={{ ['--tile' as string]: '#f08c00' }}><div className="tile-head"><span className="tile-ico"><Icon name="gift" size={14} /></span>Offers</div><div className="tile-val">{s.offers}</div><div className="muted small">to decide</div></Link>
          <Link to="/reports" className="tile"><div className="tile-head"><span className="tile-ico"><Icon name="chart" size={14} /></span>They answer</div><div className="tile-val">{s.applied_total >= 3 ? `${s.response_rate}%` : '—'}</div><div className="muted small">of applications (90 days)</div></Link>
        </div>
      </div>}

      {v.interviews.length > 0 && <section className="day-sec">
        <h3 className="day-h"><Icon name="video" size={15} />Interviews coming<span className="count">{v.interviews.length}</span></h3>
        <div className="card list">
          {v.interviews.slice(0, 6).map(iv => {
            const k = interviewKind(iv.kind)
            const isToday = iv.starts_at.slice(0, 10) === t
            return (
              <Link key={iv.id} to={`/jobs/${iv.job_id}`} className="list-row job-row">
                <span className={`list-ico ${isToday ? 'st-due' : 'st-ok'}`}><Icon name={k.icon} size={16} /></span>
                <span className="row-main grow"><span className="row-title"><span className="ellipsis">{jobLabel({ title: iv.job_title || '', company: iv.company || '' })}</span></span>
                  <span className="muted small ellipsis">{k.label}{iv.with_people && ` with ${iv.with_people}`}{iv.questions.length ? ` · ${iv.questions.length} questions ready` : ' · not prepared yet'}</span></span>
                <span className={`pill ${isToday ? 'st-due' : 'st-ok'}`}>{isToday ? `Today ${iv.starts_at.slice(11, 16)}` : whenText(iv.starts_at)}</span>
              </Link>
            )
          })}
        </div>
      </section>}

      <JobList title="Overdue" icon="alert" tone="bad" jobs={L.overdue} onStep={setStep} />
      <JobList title="Today" icon="today" jobs={L.today} onStep={setStep} empty={s.open && !L.overdue.length ? 'Nothing for today.' : undefined} />
      <JobList title="This week" icon="calendar" jobs={L.week} onStep={setStep} />
      <JobList title="Follow up" sub="Applied a while ago and no answer yet. A short, friendly email often gets things moving." icon="clock" jobs={L.follow_up} onFollow={setFollow} follow />
      {v.offers.length > 0 && <section className="day-sec">
        <h3 className="day-h"><Icon name="gift" size={15} />Offers to decide<span className="count">{v.offers.length}</span></h3>
        <div className="card list">
          {v.offers.map(o => (
            <Link key={o.id} to={`/jobs/${o.job_id}`} className="list-row job-row">
              <span className="row-main grow"><span className="row-title"><span className="ellipsis">{jobLabel({ title: o.job_title || '', company: o.company || '' })}</span></span>
                <span className="muted small">{o.base ? `${fmtMoney(o.yearly, o.currency, true)} a year` : 'Pay not written'}{o.status === 'negotiating' ? ' · negotiating' : ''}</span></span>
              {o.decide_by && <span className={`pill ${o.decide_by <= t ? 'st-overdue' : 'st-due'}`}>decide {dayWords(o.decide_by, t)}</span>}
            </Link>
          ))}
        </div>
        {v.offers.length > 1 && <Link to="/offers" className="small">Compare them side by side</Link>}
      </section>}
      <JobList title="Apply soon" sub="Saved jobs you want most, or that close soon." icon="flag" jobs={L.apply} apply />
      <JobList title="No next step" icon="target" jobs={L.no_step} onStep={setStep} quiet />
      <JobList title="Hired lately" icon="trophy" tone="good" jobs={L.recent} quiet />
      {step && <StepSheet job={step} onClose={() => setStep(null)} onDone={() => { setStep(null); load() }} />}
      {follow && <MakeSheet job={follow} target="email" emailKind="follow_up" onClose={() => setFollow(null)} onDone={() => { setFollow(null); load() }} />}
    </div>
  )
}

function JobList({ title, sub, icon, jobs, onStep, onFollow, tone = '', empty, quiet = false, follow = false, apply = false }: { title: string; sub?: string; icon: string; jobs: Job[]; onStep?: (j: Job) => void; onFollow?: (j: Job) => void; tone?: '' | 'bad' | 'good'; empty?: string; quiet?: boolean; follow?: boolean; apply?: boolean }) {
  if (!jobs.length && !empty) return null
  const t = todayISO()
  return (
    <section className={`day-sec ${tone}`}>
      <h3 className="day-h"><Icon name={icon} size={15} />{title}{jobs.length > 0 && <span className="count">{jobs.length}</span>}</h3>
      {sub && jobs.length > 0 && <p className="muted small">{sub}</p>}
      {!jobs.length ? <p className="muted small">{empty}</p> : (
        <div className="card list">
          {jobs.slice(0, 10).map(j => (
            <div key={j.id} className="list-row job-row">
              <Link to={`/jobs/${j.id}`} className="row-main grow">
                <span className="row-title"><span className="ellipsis">{quiet || follow || apply ? jobLabel(j) : (j.next_action || jobLabel(j))}</span></span>
                <span className="muted small ellipsis">
                  {quiet || follow || apply ? (j.location || j.stage_name) : jobLabel(j)}
                  {follow && j.applied_on && <> · applied {dayWords(j.applied_on, t)}</>}
                  {apply && j.apply_by && <> · apply by {fmtDate(j.apply_by, false)}</>}
                </span>
              </Link>
              {!quiet && !follow && !apply && j.next_due && j.next_due !== t && <span className={`pill ${j.next_due < t ? 'st-overdue' : 'st-ok'}`}>{fmtDate(j.next_due, false)}</span>}
              {apply && j.excitement > 0 && <span className="muted small hide-sm" title="How much you want it">{'★'.repeat(j.excitement)}</span>}
              {onStep && <button type="button" className="btn sm" onClick={() => onStep(j)} title={j.next_action ? 'Mark the step done' : 'Set the next step'}>{j.next_action && !quiet ? <><Icon name="check" size={14} /><span className="hide-sm">Done</span></> : <><Icon name="plus" size={14} /><span className="hide-sm">Step</span></>}</button>}
              {onFollow && <button type="button" className="btn sm" onClick={() => onFollow(j)}><Icon name="mail" size={14} /><span className="hide-sm">Follow up</span></button>}
            </div>
          ))}
          {jobs.length > 10 && <Link to="/jobs" className="list-row muted small">and {jobs.length - 10} more…</Link>}
        </div>
      )}
    </section>
  )
}
