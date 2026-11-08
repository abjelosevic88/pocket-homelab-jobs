// Every job in one list: open, hired, closed — with a search and filters.
import { useCallback, useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { SOURCES, STAGE_KINDS, WORK_MODES, endLabel, salaryText, stageColor } from '../lib/jobs'
import { useData, useOnChange } from '../lib/data'
import { ago, fmtDate, todayISO } from '../lib/format'
import type { Job } from '../lib/types'
import { Icon } from '../components/icons'
import { openCreate } from '../components/Layout'
import { Select } from '../components/Select'
import { Empty, Loading, Segmented, useDebounce } from '../components/ui'
import { fitTone } from './JobPage'

type StatusF = 'open' | 'hired' | 'closed' | ''

export default function Jobs() {
  const [sp, setSp] = useSearchParams()
  const { stages } = useData()
  const status = (sp.get('status') === 'all' ? '' : sp.get('status') ?? 'open') as StatusF
  const [q, setQ] = useState(sp.get('q') || '')
  const dq = useDebounce(q, 250)
  const stage = sp.get('stage') || ''
  const kind = sp.get('kind') || ''
  const tag = sp.get('tag') || ''
  const source = sp.get('source') || ''
  const mode = sp.get('work_mode') || ''
  const due = sp.get('due') || ''
  const company = sp.get('company') || ''
  const sort = sp.get('sort') || (status === 'open' ? 'stage' : 'updated')
  const [list, setList] = useState<{ items: Job[]; total: number } | null>(null)
  const set = (k: string, v: string) => { const n = new URLSearchParams(sp); if (v) n.set(k, v); else n.delete(k); setSp(n, { replace: true }) }
  const load = useCallback(async () => {
    const p = new URLSearchParams({ status, q: dq, stage, kind, tag, source, work_mode: mode, due, company, sort, limit: '500' })
    for (const [k, v] of [...p.entries()]) if (!v) p.delete(k)
    try { setList(await api.get(`${V1}/jobs?${p}`)) } catch { setList({ items: [], total: 0 }) }
  }, [status, dq, stage, kind, tag, source, mode, due, company, sort])
  useEffect(() => { load() }, [load])
  useOnChange(load)
  const t = todayISO()
  return (
    <div className="page stack">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">Jobs</h1>{list && <p className="muted">{list.total} {list.total === 1 ? 'job' : 'jobs'}</p>}</div>
        <button type="button" className="btn primary hide-sm" onClick={() => openCreate('job')}><Icon name="plus" size={15} />Add a job</button>
      </div>
      <div className="toolbar row wrap filters">
        <Segmented size="sm" label="Which" value={status} onChange={v => set('status', v === 'open' ? '' : v || 'all')} options={[{ value: 'open', label: 'Open' }, { value: 'hired', label: 'Hired' }, { value: 'closed', label: 'Closed' }, { value: '' as StatusF, label: 'All' }]} />
        <div className="search-in grow"><Icon name="search" size={15} /><input value={q} onChange={e => { setQ(e.target.value); set('q', e.target.value) }} placeholder="Search roles, companies, people…" aria-label="Search" /></div>
        <Select ariaLabel="Stage" value={stage || (kind ? 'k:' + kind : '')} onChange={v => { const n = new URLSearchParams(sp); n.delete('stage'); n.delete('kind'); if (v.startsWith('k:')) n.set('kind', v.slice(2)); else if (v) n.set('stage', v); setSp(n, { replace: true }) }} placeholder="Any stage" clearLabel="Any stage"
          options={[...(stages || []).map(s => ({ value: String(s.id), label: s.name, group: 'Stage' })), ...STAGE_KINDS.map(k => ({ value: 'k:' + k.value, label: k.label + ' (all)', group: 'Kind' }))]} />
        {status === 'open' && <Select ariaLabel="Next step" value={due} onChange={v => set('due', v)} placeholder="Any next step" clearLabel="Any next step" options={[{ value: 'overdue', label: 'Overdue' }, { value: 'today', label: 'Due today or before' }, { value: 'week', label: 'Due this week' }, { value: 'none', label: 'No next step' }]} />}
        <Select ariaLabel="Where" value={mode} onChange={v => set('work_mode', v)} placeholder="Anywhere" clearLabel="Anywhere" options={WORK_MODES} />
        <Select ariaLabel="Found on" value={source} onChange={v => set('source', v)} placeholder="Any source" clearLabel="Any source" options={SOURCES} />
        <Select ariaLabel="Sort" value={sort} onChange={v => set('sort', v)} options={[{ value: 'stage', label: 'By stage' }, { value: 'due', label: 'Next step first' }, { value: 'excitement', label: 'Most wanted' }, { value: 'fit', label: 'Best fit' }, { value: 'salary', label: 'Best paid' }, { value: 'applied', label: 'Applied lately' }, { value: 'updated', label: 'Changed lately' }, { value: 'created', label: 'Newest' }]} />
        {tag && <button type="button" className="chip on" onClick={() => set('tag', '')}>#{tag}<Icon name="x" size={12} /></button>}
        {company && <button type="button" className="chip on" onClick={() => set('company', '')}>{company}<Icon name="x" size={12} /></button>}
      </div>
      {!list ? <Loading /> : list.items.length === 0 ? <Empty title={dq || stage || kind || tag || due || source || mode ? 'Nothing matches' : status === 'open' ? 'No open jobs' : 'Nothing here yet'} icon="search">{status === 'open' && !dq && <button type="button" className="link-btn" onClick={() => openCreate('job')}>Add a job</button>}</Empty> : (
        <div className="card list">
          {list.items.map(j => {
            const sal = salaryText(j)
            return (
              <Link key={j.id} to={`/jobs/${j.id}`} className="list-row job-row">
                <span className="stage-dot" style={{ background: stageColor({ color: j.stage_color || '', kind: j.stage_kind || 'active' }) }} title={j.stage_name} />
                <span className="row-main grow">
                  <span className="row-title">{j.starred && <Icon name="star" size={12} className="star-on" />}<span className="ellipsis">{j.title || j.company}</span></span>
                  <span className="muted small ellipsis">{[j.title && j.company, j.location, j.stage_name, j.status === 'closed' && endLabel(j.end_reason)].filter(Boolean).join(' · ')}</span>
                </span>
                {j.fit_score >= 0 && <span className={`fit sm hide-sm fit-${fitTone(j.fit_score)}`} title="Fit">{j.fit_score}%</span>}
                {j.status === 'open' && (j.next_interview
                  ? <span className="pill st-due hide-sm" title="Interview">{fmtDate(j.next_interview.slice(0, 10), false)}</span>
                  : j.next_action ? <span className={`pill hide-sm ${j.overdue ? 'st-overdue' : j.next_due === t ? 'st-due' : 'st-ok'}`} title={j.next_action}>{j.next_due ? fmtDate(j.next_due, false) : 'No day'}</span>
                    : j.stale ? <span className="pill st-overdue hide-sm">Follow up</span> : null)}
                {j.status !== 'open' && j.closed_at && <span className="muted small hide-sm">{ago(j.closed_at)}</span>}
                <span className="row-amount tnum muted small hide-sm">{sal}</span>
              </Link>
            )
          })}
        </div>
      )}
    </div>
  )
}
