// Every interview: the ones coming (with how ready you are) and the ones
// past (with how they went and whether you thanked them).
import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { interviewKind, jobLabel, whenText } from '../lib/jobs'
import { useOnChange } from '../lib/data'
import { todayISO } from '../lib/format'
import type { Interview } from '../lib/types'
import { Icon } from '../components/icons'
import { Empty, Loading, Segmented } from '../components/ui'

export default function Interviews() {
  const [when, setWhen] = useState<'upcoming' | 'past'>('upcoming')
  const [list, setList] = useState<Interview[] | null>(null)
  const load = useCallback(async () => {
    try { setList(await api.get<Interview[]>(`${V1}/interviews?when=${when}&limit=300`)) } catch { setList([]) }
  }, [when])
  useEffect(() => { setList(null); load() }, [load])
  useOnChange(load)
  const t = todayISO()
  const groups: { label: string; items: Interview[] }[] = []
  for (const iv of list || []) {
    const d = iv.starts_at.slice(0, 10)
    const label = d === t ? 'Today' : when === 'upcoming' && d <= new Date(Date.now() + 6 * 86400000).toISOString().slice(0, 10) ? 'This week' : when === 'upcoming' ? 'Later' : new Date(d + 'T00:00').toLocaleDateString(undefined, { month: 'long', year: 'numeric' })
    let g = groups.find(x => x.label === label)
    if (!g) groups.push(g = { label, items: [] })
    g.items.push(iv)
  }
  return (
    <div className="page stack">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">Interviews</h1><p className="muted">Plan them from a job’s page; prepare the questions with Pocket AI.</p></div>
        <Segmented size="sm" label="Which" value={when} onChange={setWhen} options={[{ value: 'upcoming', label: 'Coming' }, { value: 'past', label: 'Past' }]} />
      </div>
      {!list ? <Loading /> : !list.length ? <Empty title={when === 'upcoming' ? 'No interviews coming' : 'No interviews yet'} icon="video">When a company invites you, open the job and plan the interview: it goes in your calendar, and you can prepare the questions there.</Empty> : groups.map(g => (
        <section key={g.label} className="day-sec">
          <h3 className="day-h">{g.label}<span className="count">{g.items.length}</span></h3>
          <div className="card list">
            {g.items.map(iv => {
              const k = interviewKind(iv.kind)
              return (
                <Link key={iv.id} to={`/jobs/${iv.job_id}`} className="list-row job-row">
                  <span className={`list-ico ${iv.past ? (iv.outcome === 'good' ? 'st-done' : iv.outcome === 'bad' ? 'st-overdue' : '') : iv.starts_at.slice(0, 10) === t ? 'st-due' : 'st-ok'}`}><Icon name={k.icon} size={16} /></span>
                  <span className="row-main grow">
                    <span className="row-title"><span className="ellipsis">{jobLabel({ title: iv.job_title || '', company: iv.company || '' })}</span></span>
                    <span className="muted small ellipsis">{[k.label, iv.with_people && `with ${iv.with_people}`, !iv.past && (iv.questions.length ? `${iv.questions.length} questions ready` : 'not prepared yet'), iv.past && (iv.outcome ? { good: 'went well', okay: 'okay', bad: 'not great' }[iv.outcome] : 'how did it go?'), iv.past && iv.outcome && !iv.thanks_sent && iv.job_status === 'open' && 'no thank-you yet'].filter(Boolean).join(' · ')}</span>
                  </span>
                  {iv.event_ref && <Icon name="calendar-check" size={14} className="muted hide-sm" />}
                  <span className="pill st-ok">{whenText(iv.starts_at)}</span>
                </Link>
              )
            })}
          </div>
        </section>
      ))}
    </div>
  )
}
