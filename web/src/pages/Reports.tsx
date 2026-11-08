// How the search goes: applications and interviews week by week, how far
// applications get, which sources and kinds of job answer, and why they end.
import { useEffect, useState } from 'react'
import { api, V1 } from '../lib/api'
import { endLabel, modeLabel, sourceLabel } from '../lib/jobs'
import type { CountRow, Reports as R } from '../lib/types'
import { Bars, HBars } from '../components/Chart'
import { Icon } from '../components/icons'
import { Loading, Segmented } from '../components/ui'

const pct = (a: number, b: number) => b ? `${Math.round((a * 100) / b)}%` : '—'

export default function Reports() {
  const [weeks, setWeeks] = useState(12)
  const [r, setR] = useState<R | null>(null)
  useEffect(() => { api.get<R>(`${V1}/reports?weeks=${weeks}`).then(setR).catch(() => setR(null)) }, [weeks])
  if (!r) return <Loading />
  const s = r.summary
  const label = (w: string) => new Date(w + 'T00:00').toLocaleDateString(undefined, { day: 'numeric', month: 'short' })
  const applied = r.funnel[0]?.count || 0
  return (
    <div className="page stack loose">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">How it goes</h1><p className="muted">{s.applied_total} applications so far{s.applied_month ? `, ${s.applied_month} this month` : ''}.</p></div>
        <Segmented size="sm" label="Weeks" value={weeks} onChange={setWeeks} options={[{ value: 8, label: '8 weeks' }, { value: 12, label: '12 weeks' }, { value: 26, label: 'Half a year' }]} />
      </div>
      <div className="tiles">
        <div className="tile static"><div className="tile-head"><span className="tile-ico"><Icon name="send" size={14} /></span>They answer</div><div className="tile-val">{applied >= 3 ? `${s.response_rate}%` : '—'}</div><div className="muted small">of applications in 90 days</div></div>
        <div className="tile static"><div className="tile-head"><span className="tile-ico"><Icon name="video" size={14} /></span>Lead to an interview</div><div className="tile-val">{applied >= 3 ? `${s.interview_rate}%` : '—'}</div><div className="muted small">of applications in 90 days</div></div>
        <div className="tile static"><div className="tile-head"><span className="tile-ico"><Icon name="hourglass" size={14} /></span>First answer</div><div className="tile-val">{r.days_to_answer ? `${r.days_to_answer} days` : '—'}</div><div className="muted small">after applying, on average</div></div>
        <div className="tile static"><div className="tile-head"><span className="tile-ico"><Icon name="target" size={14} /></span>This week</div><div className="tile-val">{s.applied_week}{s.weekly_goal ? <span className="tile-unit">of {s.weekly_goal}</span> : null}</div><div className="muted small">applications</div></div>
      </div>

      <section className="card pad-card">
        <h3 className="card-h">Applications each week</h3>
        <Bars items={r.weeks.map(w => ({ label: label(w.week), value: w.applied, hint: `${w.interviews} interviews, ${w.answers} answers` }))} height={150} />
        <h3 className="card-h mt">Interviews each week</h3>
        <Bars items={r.weeks.map(w => ({ label: label(w.week), value: w.interviews }))} color="#7048e8" height={100} />
      </section>

      <div className="two-col">
        <section className="card pad-card">
          <h3 className="card-h">How far applications get</h3>
          {applied ? <HBars items={r.funnel.map((f, i) => ({ label: f.name, value: f.count, color: ['#3d5a98', '#0c8599', '#7048e8', '#f08c00', '#2f9e44'][i], hint: `${pct(f.count, applied)} of applications` }))} /> : <p className="muted small">Nothing applied for yet.</p>}
        </section>
        <section className="card pad-card">
          <h3 className="card-h">Why jobs ended</h3>
          {r.end_reasons.length ? <HBars items={r.end_reasons.map(x => ({ label: endLabel(x.name), value: x.count, color: 'var(--danger)' }))} /> : <p className="muted small">None ended yet.</p>}
        </section>
      </div>
      <Table title="Where you found them" rows={r.sources} name={sourceLabel} />
      <Table title="On site, hybrid or remote" rows={r.work_modes} name={v => modeLabel(v) || 'Not said'} />
    </div>
  )
}

function Table({ title, rows, name }: { title: string; rows: CountRow[]; name: (v: string) => string }) {
  if (!rows.length) return null
  return (
    <section className="card pad-card">
      <h3 className="card-h">{title}</h3>
      <div className="scroll-x"><table className="table compact">
        <thead><tr><th></th><th className="right">Applied</th><th className="right">Answered</th><th className="right">Interviews</th><th className="right">Offers</th></tr></thead>
        <tbody>{rows.map(x => <tr key={x.name}><td>{x.name === 'not said' ? <span className="muted">Not said</span> : name(x.name)}</td><td className="right tnum">{x.count}</td><td className="right tnum">{x.answered} <span className="muted tiny">{pct(x.answered, x.count)}</span></td><td className="right tnum">{x.interviews}</td><td className="right tnum">{x.offers}</td></tr>)}</tbody>
      </table></div>
    </section>
  )
}
