// The board: one column per open stage (saved → offer), cards you drag
// between them (or move with the card's menu on a phone); drop on Hired or
// Closed to end one.
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { OPEN_KINDS, modeLabel, salaryText, stageColor, whenText } from '../lib/jobs'
import { announceChange, useData, useOnChange } from '../lib/data'
import { fmtDate, todayISO } from '../lib/format'
import type { Job, Stage } from '../lib/types'
import { CloseSheet, HiredSheet } from '../components/JobActions'
import { Icon } from '../components/icons'
import { openCreate } from '../components/Layout'
import { Loading, type MenuItem, MenuButton, errMsg, useIsMobile, useToast } from '../components/ui'
import { fitTone } from './JobPage'

export default function Board() {
  const { stages } = useData()
  const { toast } = useToast()
  const navigate = useNavigate()
  const mobile = useIsMobile()
  const [jobs, setJobs] = useState<Job[] | null>(null)
  const [drag, setDrag] = useState<number>(0)
  const [over, setOver] = useState<{ stage: number; index: number } | null>(null)
  const [ending, setEnding] = useState<{ job: Job; kind: 'hired' | 'closed' } | null>(null)
  const load = useCallback(async () => {
    try { setJobs((await api.get<{ items: Job[] }>(`${V1}/jobs?status=open&sort=stage&limit=1000`)).items) } catch { setJobs(d => d || []) }
  }, [])
  useEffect(() => { load() }, [load])
  useOnChange(load)
  const open = useMemo(() => (stages || []).filter(s => OPEN_KINDS.includes(s.kind)), [stages])
  const byStage = useMemo(() => {
    const m: Record<number, Job[]> = {}
    for (const s of open) m[s.id] = []
    for (const j of jobs || []) (m[j.stage_id] ||= []).push(j)
    for (const k in m) m[k].sort((a, b) => a.position - b.position || a.id - b.id)
    return m
  }, [jobs, open])
  const hired = (stages || []).find(s => s.kind === 'hired')
  const closed = (stages || []).find(s => s.kind === 'closed')

  const move = async (j: Job, stage: Stage, index: number) => {
    const col = (byStage[stage.id] || []).filter(x => x.id !== j.id)
    const before = index > 0 ? col[index - 1]?.position : undefined
    const after = col[index]?.position
    setJobs(js => (js || []).map(x => x.id === j.id ? { ...x, stage_id: stage.id, stage_name: stage.name, position: before !== undefined && after !== undefined ? (before + after) / 2 : before !== undefined ? before + 1 : after !== undefined ? after - 1 : 0 } : x))
    try { await api.post(`${V1}/jobs/${j.id}/move`, { stage_id: stage.id, before, after }); announceChange() } catch (e) { toast(errMsg(e), 'err'); load() }
  }
  const dropOn = (stage: Stage, index: number) => {
    const j = (jobs || []).find(x => x.id === drag)
    setDrag(0); setOver(null)
    if (!j) return
    const col = byStage[stage.id] || []
    const from = col.findIndex(x => x.id === j.id)
    if (from >= 0 && (from === index || from === index - 1)) return
    move(j, stage, from >= 0 && from < index ? index - 1 : index)
  }
  if (!stages || !jobs) return <Loading />
  const t = todayISO()
  return (
    <div className="board-page">
      <div className="page-head board-head">
        <div className="grow">
          <h1 className="page-title">Board</h1>
          <p className="muted">{jobs.length} open {jobs.length === 1 ? 'job' : 'jobs'}{jobs.filter(j => j.next_interview).length ? ` · ${jobs.filter(j => j.next_interview).length} with an interview coming` : ''}</p>
        </div>
        <Link to="/settings/stages" className="btn ghost"><Icon name="sliders" size={15} /><span className="hide-sm">Stages</span></Link>
        <button type="button" className="btn primary hide-sm" onClick={() => openCreate('job')}><Icon name="plus" size={15} />Add a job</button>
      </div>
      <div className="board">
        {open.map(s => {
          const col = byStage[s.id] || []
          return (
            <section key={s.id} className={`col ${drag ? 'dragging' : ''} ${over?.stage === s.id ? 'over' : ''}`} style={{ ['--st' as string]: stageColor(s) }}
              onDragOver={e => { if (!drag) return; e.preventDefault(); if (over?.stage !== s.id && !col.length) setOver({ stage: s.id, index: 0 }) }}
              onDrop={e => { e.preventDefault(); dropOn(s, over?.stage === s.id ? over.index : col.length) }}>
              <header className="col-head">
                <span className="col-dot" />
                <span className="col-name ellipsis">{s.name}</span>
                <span className="count">{col.length}</span>
              </header>
              <div className="col-cards">
                {col.map((j, i) => (
                  <div key={j.id} className="card-slot"
                    onDragOver={e => { if (!drag) return; e.preventDefault(); e.stopPropagation(); const r = e.currentTarget.getBoundingClientRect(); const idx = e.clientY < r.top + r.height / 2 ? i : i + 1; if (over?.stage !== s.id || over.index !== idx) setOver({ stage: s.id, index: idx }) }}
                    onDrop={e => { e.preventDefault(); e.stopPropagation(); dropOn(s, over?.stage === s.id ? over.index : i) }}>
                    {over?.stage === s.id && over.index === i && drag !== j.id && <div className="drop-line" />}
                    <JobCard j={j} t={t} dragging={drag === j.id} draggable={!mobile}
                      onDragStart={() => setDrag(j.id)} onDragEnd={() => { setDrag(0); setOver(null) }}
                      onOpen={() => navigate(`/jobs/${j.id}`)}
                      menu={(): MenuItem[] => [
                        { heading: 'Move to' },
                        ...open.filter(x => x.id !== s.id).map(x => ({ label: x.name, icon: 'move', onSelect: () => move(j, x, 0) })),
                        'sep',
                        ...(hired ? [{ label: 'Hired', icon: 'trophy', onSelect: () => setEnding({ job: j, kind: 'hired' }) }] : []),
                        ...(closed ? [{ label: 'Close it', icon: 'x-circle', onSelect: () => setEnding({ job: j, kind: 'closed' }) }] : []),
                      ]} />
                    {over?.stage === s.id && over.index === col.length && i === col.length - 1 && <div className="drop-line" />}
                  </div>
                ))}
                {col.length === 0 && <div className="col-empty muted small">{drag ? 'Drop here' : 'Nothing here'}</div>}
                <button type="button" className="col-add" onClick={() => openCreate('job', { stage_id: s.id })}><Icon name="plus" size={14} />Add</button>
              </div>
            </section>
          )
        })}
        {drag > 0 && <div className="close-zones">
          {hired && <div className="close-zone won" onDragOver={e => e.preventDefault()} onDrop={e => { e.preventDefault(); const j = jobs.find(x => x.id === drag); setDrag(0); setOver(null); if (j) setEnding({ job: j, kind: 'hired' }) }}><Icon name="trophy" size={18} />Hired</div>}
          {closed && <div className="close-zone lost" onDragOver={e => e.preventDefault()} onDrop={e => { e.preventDefault(); const j = jobs.find(x => x.id === drag); setDrag(0); setOver(null); if (j) setEnding({ job: j, kind: 'closed' }) }}><Icon name="x-circle" size={18} />Closed</div>}
        </div>}
      </div>
      {ending?.kind === 'hired' && <HiredSheet job={ending.job} onClose={() => setEnding(null)} onDone={() => { setEnding(null); load() }} />}
      {ending?.kind === 'closed' && <CloseSheet job={ending.job} onClose={() => setEnding(null)} onDone={() => { setEnding(null); load() }} />}
    </div>
  )
}

function JobCard({ j, t, dragging, draggable, onDragStart, onDragEnd, onOpen, menu }: { j: Job; t: string; dragging: boolean; draggable: boolean; onDragStart: () => void; onDragEnd: () => void; onOpen: () => void; menu: () => MenuItem[] }) {
  const salary = salaryText(j)
  return (
    <article className={`job-card ${dragging ? 'is-dragging' : ''} ${j.overdue ? 'overdue' : ''}`} draggable={draggable}
      onDragStart={e => { e.dataTransfer.effectAllowed = 'move'; e.dataTransfer.setData('text/plain', String(j.id)); onDragStart() }} onDragEnd={onDragEnd}
      onClick={e => { if (!(e.target as HTMLElement).closest('button, a')) onOpen() }} role="link" tabIndex={0} onKeyDown={e => { if (e.key === 'Enter') onOpen() }}>
      <div className="dc-top">
        <span className="dc-title">{j.starred && <Icon name="star" size={12} className="star-on" />}{j.title || j.company}</span>
        <MenuButton label="Move" items={menu} className="btn ghost icon-only sm dc-menu" align="right" width={210}><Icon name="more" size={15} /></MenuButton>
      </div>
      {j.title && j.company && <div className="dc-who ellipsis">{j.company}{j.location || j.work_mode ? <span className="muted"> · {j.work_mode === 'remote' ? 'Remote' : j.location || modeLabel(j.work_mode)}</span> : null}</div>}
      {salary && <div className="dc-sal tnum ellipsis">{salary}</div>}
      <div className="dc-foot">
        {j.fit_score >= 0 && <span className={`fit sm fit-${fitTone(j.fit_score)}`} title="How well your CV fits">{j.fit_score}%</span>}
        {j.has_offer && <span className="dc-tag offer" title="An offer"><Icon name="gift" size={11} />Offer</span>}
        <span className="spacer" />
        {j.next_interview
          ? <span className="dc-step iv" title="The next interview"><Icon name="video" size={11} />{whenText(j.next_interview)}</span>
          : j.next_action
            ? <span className={`dc-step ${j.overdue ? 'late' : j.next_due === t ? 'today' : ''}`} title={j.next_action}><Icon name="target" size={11} />{j.next_due ? (j.next_due === t ? 'Today' : fmtDate(j.next_due, false)) : 'No day'}</span>
            : j.stale ? <span className="dc-step late" title="No answer for a while: follow up"><Icon name="clock" size={11} />Follow up</span>
              : j.apply_by ? <span className={`dc-step ${j.apply_by < t ? 'late' : ''}`} title="Apply by"><Icon name="flag" size={11} />{fmtDate(j.apply_by, false)}</span> : null}
      </div>
    </article>
  )
}
