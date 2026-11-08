// Left column: add a job (and the import), Today, the board and lists, then
// the open stages with their jobs. The Pocket Calendar sidebar.
import { useState } from 'react'
import { Link, NavLink, useLocation, useNavigate } from 'react-router-dom'
import { useApp } from '../lib/app-context'
import { stageColor } from '../lib/jobs'
import { useData } from '../lib/data'
import { Icon } from './icons'
import { createItems, type CreateKind } from './Layout'
import { type Anchor, Menu, rectOf } from './ui'

export function Sidebar({ onCreate, onNavigate }: { onCreate: (k: CreateKind) => void; onNavigate: () => void }) {
  const { status, settings } = useApp()
  const { stages, summary } = useData()
  const navigate = useNavigate()
  const loc = useLocation()
  const [menu, setMenu] = useState<Anchor | null>(null)
  const title = settings?.title || status?.title || 'Pocket Jobs'
  const sp = new URLSearchParams(loc.search)
  const curStage = loc.pathname === '/jobs' ? Number(sp.get('stage') || 0) : 0
  const due = (summary?.due_today || 0) + (summary?.overdue || 0)
  const nav: { to: string; label: string; icon: string; active: boolean; count?: number; hint?: string }[] = [
    { to: '/', label: 'Today', icon: 'today', active: loc.pathname === '/', count: due, hint: 'Next steps due today or overdue' },
    { to: '/board', label: 'Board', icon: 'layers', active: loc.pathname === '/board' },
    { to: '/jobs', label: 'Jobs', icon: 'briefcase', active: (loc.pathname === '/jobs' && !curStage) || /^\/jobs\/\d/.test(loc.pathname) },
    { to: '/interviews', label: 'Interviews', icon: 'video', active: loc.pathname === '/interviews', count: summary?.interviews_upcoming, hint: 'Interviews coming up' },
    { to: '/offers', label: 'Offers', icon: 'gift', active: loc.pathname === '/offers', count: summary?.offers, hint: 'Offers to decide' },
    { to: '/cvs', label: 'CVs & letters', icon: 'file-text', active: loc.pathname === '/cvs' },
    { to: '/companies', label: 'Companies', icon: 'building', active: loc.pathname === '/companies' },
    { to: '/reports', label: 'How it goes', icon: 'chart', active: loc.pathname === '/reports' },
  ]
  const open = (stages || []).filter(s => !['hired', 'closed'].includes(s.kind))
  return (
    <aside className="sidebar" aria-label="Pocket Jobs">
      <div className="sb-brand">
        <img src="/favicon.svg" alt="" className="sb-logo" />
        <span className="sb-title">{title !== 'Pocket Jobs' ? title : <>Pocket <b>Jobs</b></>}</span>
        {status?.version && <span className="sb-version" title={`Version ${status.version}`}>{/^v?\d+\.\d+/.test(status.version) ? 'v' + status.version.replace(/^v/, '').replace(/[-+].*$/, '') : status.version}</span>}
      </div>
      <div className="create-split">
        <button type="button" className="btn primary create-btn" onClick={() => onCreate('job')} title="Add a job (N)">
          <Icon name="plus" size={15} strokeWidth={2.2} />Add a job
        </button>
        <button type="button" className="btn primary create-more" aria-label="More ways to start" aria-haspopup="menu" aria-expanded={!!menu} onClick={e => setMenu(menu ? null : rectOf(e.currentTarget))}>
          <Icon name="chevron-down" size={15} />
        </button>
      </div>
      {menu && <Menu anchor={menu} items={createItems(onCreate)} onClose={() => setMenu(null)} width={300} />}

      <nav className="sb-nav" aria-label="Sections">
        {nav.map(n => (
          <NavLink key={n.to} to={n.to} onClick={onNavigate} title={n.hint} className={() => `sb-nav-item ${n.active ? 'active' : ''}`}>
            <Icon name={n.icon} size={16} />{n.label}
            {!!n.count && <span className="sb-count" title={n.hint}>{n.count}</span>}
          </NavLink>
        ))}
      </nav>

      {open.length > 0 && <section className="sb-sec">
        <h4 className="sb-h">Stages</h4>
        {open.map(s => (
          <button key={s.id} type="button" className={`sb-row sb-list ${curStage === s.id ? 'active' : ''}`} onClick={() => { onNavigate(); navigate(`/jobs?stage=${s.id}`) }}>
            <span className="sb-stage-dot" style={{ background: stageColor(s) }} aria-hidden="true" />
            <span className="grow ellipsis">{s.name}</span>
            <span className="muted small tnum">{s.jobs || ''}</span>
          </button>
        ))}
      </section>}
      {stages && summary && summary.open === 0 && summary.hired + summary.closed === 0 && <section className="sb-sec">
        <button type="button" className="sb-row sb-list" onClick={() => onCreate('job')}><span className="sb-list-ico" aria-hidden="true"><Icon name="plus" size={14} /></span><span className="grow">Add your first job</span></button>
      </section>}

      <div className="sb-foot">
        <Link to="/settings" className="sb-foot-link" onClick={onNavigate}><Icon name="settings" size={15} />Settings</Link>
        <button type="button" className="sb-foot-link" onClick={() => { onNavigate(); window.dispatchEvent(new CustomEvent('pjb:shortcuts')) }}><Icon name="keyboard" size={15} />Shortcuts</button>
      </div>
    </aside>
  )
}
