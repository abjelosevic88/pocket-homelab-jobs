// Companies: where you applied, the roles, who you talked to and how it went.
import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useAppState } from '../lib/data'
import { ago } from '../lib/format'
import type { Company } from '../lib/types'
import { Icon } from '../components/icons'
import { Empty, Loading, useDebounce } from '../components/ui'

export default function Companies() {
  const [sp, setSp] = useSearchParams()
  const [q, setQ] = useState(sp.get('q') || '')
  const dq = useDebounce(q, 200)
  const [list, setList] = useState<Company[] | null>(null)
  const appState = useAppState()
  const contacts = appState('contacts', 'contacts:read')
  useEffect(() => {
    setSp(dq ? { q: dq } : {}, { replace: true })
    api.get<Company[]>(`${V1}/companies${dq ? `?q=${encodeURIComponent(dq)}` : ''}`).then(setList).catch(() => setList([]))
  }, [dq]) // eslint-disable-line react-hooks/exhaustive-deps
  const cardURL = (ref: string) => contacts.app?.open_url && ref ? `${contacts.app.open_url.replace(/\/$/, '')}/contacts/${ref}` : ''
  return (
    <div className="page stack">
      <div className="page-head"><div className="grow"><h1 className="page-title">Companies</h1>{list && <p className="muted">{list.length} {list.length === 1 ? 'company' : 'companies'}</p>}</div></div>
      <div className="search-in"><Icon name="search" size={15} /><input value={q} onChange={e => setQ(e.target.value)} placeholder="Find a company, a role, a person…" aria-label="Search" /></div>
      {!list ? <Loading /> : !list.length ? <Empty title={dq ? 'Nothing matches' : 'No companies yet'} icon="building">The companies of your jobs show here, with the roles and the people you talked to.</Empty> : (
        <div className="card list">
          {list.map(c => (
            <div key={c.key} className="list-row job-row">
              <span className="list-ico"><Icon name="building" size={16} /></span>
              <Link to={c.jobs === 1 ? `/jobs/${c.job_ids[0]}` : `/jobs?status=all&company=${encodeURIComponent(c.name)}`} className="row-main grow">
                <span className="row-title"><span className="ellipsis">{c.name}</span></span>
                <span className="muted small ellipsis">{[c.roles.slice(0, 2).join(', '), c.people.length && `with ${c.people.slice(0, 2).join(', ')}`, c.interviews && `${c.interviews} ${c.interviews === 1 ? 'interview' : 'interviews'}`].filter(Boolean).join(' · ')}</span>
              </Link>
              <span className="muted small hide-sm">{[c.open && `${c.open} open`, c.hired && 'hired', c.closed && `${c.closed} closed`].filter(Boolean).join(' · ')}</span>
              <span className="muted tiny hide-sm">{c.last_at ? ago(c.last_at) : ''}</span>
              {cardURL(c.company_ref) && <a className="btn ghost icon-only sm" href={cardURL(c.company_ref)} target="_blank" rel="noreferrer" title="Open in Pocket Contacts"><Icon name="users" size={14} /></a>}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
