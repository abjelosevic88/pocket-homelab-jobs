// The app shell: sidebar, top bar, search palette, shortcuts — the Pocket Calendar layout.
import { createContext, useCallback, useContext, useEffect, useRef, useState } from 'react'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { DataProvider, useData } from '../lib/data'
import { Icon } from './icons'
import { Sidebar } from './Sidebar'
import { CommandPalette, type PaletteCommand } from './CommandPalette'
import { ShortcutsSheet } from './ShortcutsSheet'
import { ImportModal } from './ImportModal'
import { JobForm } from './JobForm'
import type { Job } from '../lib/types'
import { Avatar, Loading, type MenuItem, MenuButton, modalOpen, useIsMobile, useMediaQuery } from './ui'
import { AskButton } from './AskButton'

export { announceChange, useOnChange } from '../lib/data'

const SIDEBAR_KEY = 'pjb-sidebar'

/** The page's own name for the top bar breadcrumb. */
const CrumbCtx = createContext<(t: string) => void>(() => {})
export function useCrumb(title: string | undefined) {
  const set = useContext(CrumbCtx)
  useEffect(() => { set(title || ''); return () => set('') }, [title, set])
}

export type CreateKind = 'job' | 'import'

/** Start something from anywhere (a page's button), with what is known already. */
export const openCreate = (k: CreateKind, initial?: Partial<Job>) => window.dispatchEvent(new CustomEvent('pjb:create', { detail: { kind: k, initial } }))

/** The add-a-job menu: the sidebar's caret. */
export function createItems(onCreate: (k: CreateKind) => void): MenuItem[] {
  return [
    { label: 'Add a job', icon: 'plus', hint: 'N', onSelect: () => onCreate('job') },
    'sep',
    { label: 'Bring jobs from Huntr, Teal or a spreadsheet…', icon: 'upload', onSelect: () => onCreate('import') },
  ]
}

export default function Layout() {
  return <DataProvider><Shell /></DataProvider>
}

function Shell() {
  const navigate = useNavigate()
  const loc = useLocation()
  const { stages } = useData()
  const mobile = useIsMobile()
  const narrow = useMediaQuery('(max-width: 1099px)')
  const drawer = mobile || narrow
  const [sidebarOpen, setSidebarOpen] = useState(() => !drawer && localStorage.getItem(SIDEBAR_KEY) !== '0')
  useEffect(() => { setSidebarOpen(!drawer && localStorage.getItem(SIDEBAR_KEY) !== '0') }, [drawer])
  const drawerOpenedAt = useRef(0)
  const toggleSidebar = () => setSidebarOpen(o => { drawerOpenedAt.current = performance.now(); const n = !o; if (!drawer) localStorage.setItem(SIDEBAR_KEY, n ? '1' : '0'); return n })
  const closeDrawer = () => { if (drawer) setSidebarOpen(false) }
  useEffect(() => { document.querySelector('.content')?.scrollTo({ top: 0 }) }, [loc.pathname])

  const [crumb, setCrumbRaw] = useState('')
  const setCrumb = useCallback((t: string) => setCrumbRaw(t), [])

  const [palette, setPalette] = useState(false)
  const [shortcuts, setShortcuts] = useState(false)
  const [importing, setImporting] = useState(false)
  const [newJob, setNewJob] = useState<Partial<Job> | null>(null)

  const create = (k: CreateKind, initial?: Partial<Job>) => {
    closeDrawer()
    if (k === 'job') setNewJob(initial || {})
    else setImporting(true)
  }
  // /jobs/new?url=&title=&company=&name=&email= (a link shared from the phone, another app) opens the form filled in
  useEffect(() => {
    if (loc.pathname !== '/jobs/new') return
    const sp = new URLSearchParams(loc.search)
    const shared = sp.get('url') || sp.get('text') || ''
    const url = (shared.match(/https?:\/\/\S+/) || [''])[0]
    setNewJob({ url, title: sp.get('title') || '', company: sp.get('company') || '', contact_name: sp.get('name') || '', contact_email: sp.get('email') || '' })
    navigate('/board', { replace: true })
  }, [loc.pathname]) // eslint-disable-line react-hooks/exhaustive-deps
  const createRef = useRef(create)
  createRef.current = create

  useEffect(() => {
    const open = () => setShortcuts(true)
    const make = (e: Event) => { const d = (e as CustomEvent<{ kind: CreateKind; initial?: Partial<Job> }>).detail; createRef.current(d.kind, d.initial) }
    window.addEventListener('pjb:shortcuts', open)
    window.addEventListener('pjb:create', make)
    return () => { window.removeEventListener('pjb:shortcuts', open); window.removeEventListener('pjb:create', make) }
  }, [])
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); setPalette(p => !p); return }
      if (e.metaKey || e.ctrlKey || e.altKey) return
      if (t.closest('input, textarea, select, [contenteditable="true"]') || modalOpen() || document.querySelector('.menu, .popover, .sel-pop')) return
      const go = (to: string) => { e.preventDefault(); navigate(to) }
      const mk = (k: CreateKind) => { e.preventDefault(); createRef.current(k) }
      switch (e.key) {
        case '/': e.preventDefault(); setPalette(true); break
        case '?': e.preventDefault(); setShortcuts(true); break
        case 'n': mk('job'); break
        case 'h': go('/'); break
        case 'b': go('/board'); break
        case 'j': go('/jobs'); break
        case 'i': go('/interviews'); break
        case 'o': go('/offers'); break
        case 'v': go('/cvs'); break
        case 'c': go('/companies'); break
        case 'r': go('/reports'); break
        case ',': go('/settings'); break
        case '[': e.preventDefault(); toggleSidebar(); break
      }
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }) // eslint-disable-line react-hooks/exhaustive-deps

  const commands: PaletteCommand[] = [
    { id: 'job', label: 'Add a job', icon: 'plus', hint: 'N', keywords: 'new save posting link apply', run: () => create('job') },
    { id: 'import', label: 'Bring jobs from Huntr, Teal or a spreadsheet…', icon: 'upload', keywords: 'import csv excel', run: () => create('import') },
    { id: 'today', label: 'Today', icon: 'today', hint: 'H', keywords: 'home due steps', run: () => navigate('/') },
    { id: 'board', label: 'Board', icon: 'layers', hint: 'B', keywords: 'stages kanban pipeline', run: () => navigate('/board') },
    { id: 'jobs', label: 'All jobs', icon: 'list', hint: 'J', run: () => navigate('/jobs') },
    { id: 'interviews', label: 'Interviews', icon: 'video', hint: 'I', keywords: 'calls meetings prepare', run: () => navigate('/interviews') },
    { id: 'offers', label: 'Offers', icon: 'gift', hint: 'O', keywords: 'compare salary negotiate', run: () => navigate('/offers') },
    { id: 'cvs', label: 'CVs & letters', icon: 'file-text', hint: 'V', keywords: 'resume cover letter documents', run: () => navigate('/cvs') },
    { id: 'follow', label: 'Applications to follow up', icon: 'clock', keywords: 'quiet no answer ghosted', run: () => navigate('/jobs?kind=applied&sort=applied') },
    { id: 'overdue', label: 'Overdue steps', icon: 'alert', keywords: 'late', run: () => navigate('/jobs?due=overdue') },
    { id: 'saved', label: 'Saved, not applied yet', icon: 'star', run: () => navigate('/jobs?kind=saved') },
    { id: 'closed', label: 'Closed jobs', icon: 'x-circle', keywords: 'rejected withdrew', run: () => navigate('/jobs?status=closed') },
    { id: 'companies', label: 'Companies', icon: 'building', hint: 'C', keywords: 'employers recruiters people', run: () => navigate('/companies') },
    { id: 'reports', label: 'How it goes', icon: 'chart', hint: 'R', keywords: 'reports stats response rate funnel', run: () => navigate('/reports') },
    ...(stages || []).filter(s => !['hired', 'closed'].includes(s.kind)).map(s => ({ id: `stage-${s.id}`, label: `Jobs in ${s.name}`, icon: 'move', keywords: 'stage', run: () => navigate(`/jobs?stage=${s.id}`) })),
    { id: 'settings', label: 'Settings', icon: 'settings', hint: ',', run: () => navigate('/settings') },
    { id: 'search-set', label: 'Settings: your search', icon: 'sliders', keywords: 'goal currency salary stages', run: () => navigate('/settings/general') },
    { id: 'stages-set', label: 'Settings: stages', icon: 'layers', keywords: 'board columns', run: () => navigate('/settings/stages') },
    { id: 'apps', label: 'Settings: connected apps', icon: 'link', keywords: 'contacts mail calendar tasks notes documents learn weather core', run: () => navigate('/settings/apps') },
    { id: 'ai-set', label: 'Settings: AI', icon: 'sparkles', keywords: 'pocket ai openai about me', run: () => navigate('/settings/ai') },
    { id: 'export', label: 'Settings: export your search', icon: 'download', keywords: 'backup json csv', run: () => navigate('/settings/data') },
    { id: 'shortcuts', label: 'Keyboard shortcuts', icon: 'keyboard', hint: '?', run: () => setShortcuts(true) },
  ]

  return (
    <CrumbCtx.Provider value={setCrumb}>
      <div className={`app ${sidebarOpen ? 'with-sidebar' : 'no-sidebar'} ${mobile ? 'is-mobile' : ''} ${drawer ? 'drawer-mode' : ''}`}>
        {drawer && sidebarOpen && <div className="drawer-backdrop" onClick={e => { if (e.timeStamp - drawerOpenedAt.current > 400) setSidebarOpen(false) }} />}
        <div className={`sidebar-wrap ${sidebarOpen ? 'open' : ''}`}>
          <Sidebar onCreate={create} onNavigate={closeDrawer} />
        </div>
        <div className="main">
          <TopBar onMenu={toggleSidebar} onPalette={() => setPalette(true)} mobile={mobile} compact={narrow} sidebarOpen={sidebarOpen} crumb={crumb} />
          <main className={`content ${loc.pathname === '/board' ? 'wide' : ''}`}><div className="content-inner">{stages ? <Outlet /> : <Loading />}</div></main>
        </div>
        {mobile && !/^\/(jobs\/\d|settings)/.test(loc.pathname) && <button type="button" className="fab" aria-label="Add a job" title="Add a job" onClick={() => create('job')}><Icon name="plus" size={24} strokeWidth={2.2} /></button>}
        {palette && <CommandPalette onClose={() => setPalette(false)} commands={commands} />}
        {shortcuts && <ShortcutsSheet onClose={() => setShortcuts(false)} />}
        {importing && <ImportModal onClose={() => setImporting(false)} />}
        {newJob && <JobForm initial={newJob} onClose={() => setNewJob(null)} />}
      </div>
    </CrumbCtx.Provider>
  )
}

const SECTION: Record<string, [string, string]> = { '': ['Today', '/'], board: ['Board', '/board'], jobs: ['Jobs', '/jobs'], interviews: ['Interviews', '/interviews'], offers: ['Offers', '/offers'], cvs: ['CVs & letters', '/cvs'], companies: ['Companies', '/companies'], reports: ['How it goes', '/reports'] }

function TopBar({ onMenu, onPalette, mobile, compact, sidebarOpen, crumb }: { onMenu: () => void; onPalette: () => void; mobile: boolean; compact: boolean; sidebarOpen: boolean; crumb: string }) {
  const { user, themeMode, setThemeMode, status, setUser } = useApp()
  const navigate = useNavigate()
  const loc = useLocation()
  const [section, sectionTo] = SECTION[loc.pathname.split('/')[1] || ''] || ['', '/']
  const signOut = async () => { await api.post(`${V1}/auth/logout`).catch(() => {}); setUser(null); navigate('/login') }
  const avatarMenu = (): MenuItem[] => [
    { heading: user?.name || user?.email || '' },
    { label: 'Profile', icon: 'user', onSelect: () => navigate('/settings/profile') },
    { label: 'Settings', icon: 'settings', onSelect: () => navigate('/settings'), hint: ',' },
    { label: 'Keyboard shortcuts', icon: 'keyboard', onSelect: () => window.dispatchEvent(new CustomEvent('pjb:shortcuts')), hint: '?' },
    'sep',
    { heading: 'Appearance' },
    { label: 'System', checked: themeMode === 'system', onSelect: () => setThemeMode('system') },
    { label: 'Light', checked: themeMode === 'light', onSelect: () => setThemeMode('light') },
    { label: 'Dark', checked: themeMode === 'dark', onSelect: () => setThemeMode('dark') },
    ...(status?.auth_mode === 'local' ? ['sep' as const, { label: 'Sign out', icon: 'log-out', onSelect: signOut }] : []),
  ]
  return (
    <header className="topbar">
      <button type="button" className="btn ghost icon-only" onClick={onMenu} aria-label={sidebarOpen ? 'Hide the sidebar' : 'Show the sidebar'} title="Menu ([)"><Icon name={mobile || compact ? 'menu' : 'sidebar'} size={18} /></button>
      <h1 className="range-title">
        {crumb ? <><button type="button" className="crumb" onClick={() => navigate(sectionTo)}>{section}</button><Icon name="chevron-right" size={16} className="crumb-sep" /><span className="ellipsis">{crumb}</span></> : section}
      </h1>
      <span className="spacer" />
      <button type="button" className="btn ghost icon-only" onClick={onPalette} aria-label="Search" title="Search (⌘K or /)"><Icon name="search" size={18} /></button>
      <AskButton />
      <MenuButton label="Account" className="avatar-btn" align="right" width={220} items={avatarMenu}>
        <Avatar name={user?.name} email={user?.email} size={30} />
      </MenuButton>
    </header>
  )
}
