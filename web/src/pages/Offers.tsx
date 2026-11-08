// Offers side by side: the pay counted for a year, the bonus, days off and
// what else comes with them — and Pocket AI's view on how to decide and
// negotiate.
import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { dayWords, modeLabel } from '../lib/jobs'
import { useOnChange } from '../lib/data'
import { fmtDate, fmtMoney, todayISO } from '../lib/format'
import type { Offer } from '../lib/types'
import { Stars } from '../components/JobForm'
import { useAI } from '../components/JobActions'
import { Icon } from '../components/icons'
import { Empty, Loading, Modal, Spinner, copyText, errMsg, useToast } from '../components/ui'

const STATUS: Record<Offer['status'], string> = { open: 'To decide', negotiating: 'Negotiating', accepted: 'Accepted', declined: 'Declined' }

export default function Offers() {
  const ai = useAI()
  const { toast } = useToast()
  const [list, setList] = useState<Offer[] | null>(null)
  const [advice, setAdvice] = useState('')
  const [busy, setBusy] = useState(false)
  const load = useCallback(async () => { try { setList(await api.get<Offer[]>(`${V1}/offers`)) } catch { setList([]) } }, [])
  useEffect(() => { load() }, [load])
  useOnChange(load)
  if (!list) return <Loading />
  const open = list.filter(o => o.status === 'open' || o.status === 'negotiating')
  const rest = list.filter(o => !open.includes(o))
  const best = open.length > 1 ? Math.max(...open.map(o => o.currency === open[0].currency ? o.yearly : 0)) : 0
  const t = todayISO()
  const ask = async () => {
    setBusy(true)
    try { setAdvice((await api.post<{ text: string }>(`${V1}/offers/ai`, {})).text) } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  return (
    <div className="page stack loose">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">Offers</h1><p className="muted">{open.length ? `${open.length} to decide` : 'Nothing to decide right now'}</p></div>
        {ai && open.length > 0 && <button type="button" className="btn" onClick={ask} disabled={busy}>{busy ? <Spinner /> : <Icon name="sparkles" size={15} />}Help me decide</button>}
      </div>
      {!list.length && <Empty title="No offers yet" icon="gift">When a company makes you an offer, add it on the job’s page (More → Add an offer). With two or more, this page puts them side by side.</Empty>}
      {open.length > 0 && <div className="offer-cols">
        {open.map(o => (
          <article key={o.id} className={`card offer-col ${best && o.yearly === best ? 'best' : ''}`}>
            <Link to={`/jobs/${o.job_id}`} className="offer-co"><b>{o.company || o.job_title}</b>{o.company && <span className="muted small ellipsis">{o.job_title}</span>}</Link>
            <div className="offer-big tnum">{o.base ? fmtMoney(o.yearly, o.currency, true) : '—'}<span className="muted small"> a year</span></div>
            {best > 0 && o.yearly === best && <span className="pill st-done">Pays the most</span>}
            <dl className="facts">
              <dt>Pay</dt><dd className="tnum">{o.base ? `${fmtMoney(o.base, o.currency)} ${{ year: 'a year', month: 'a month', hour: 'an hour' }[o.period]}` : 'not written'}</dd>
              <dt>Bonus</dt><dd className="tnum">{o.bonus ? fmtMoney(o.bonus, o.currency) : '—'}</dd>
              <dt>Days off</dt><dd>{o.pto_days || '—'}</dd>
              <dt>Shares</dt><dd>{o.equity || '—'}</dd>
              <dt>Where</dt><dd>{[o.location, modeLabel(o.work_mode || '')].filter(Boolean).join(' · ') || '—'}</dd>
              <dt>Start</dt><dd>{o.start_date ? fmtDate(o.start_date) : '—'}</dd>
              <dt>Want it</dt><dd><Stars value={o.excitement} size={13} /></dd>
              <dt>Decide by</dt><dd className={o.decide_by && o.decide_by <= t ? 'danger-text' : ''}>{o.decide_by ? `${fmtDate(o.decide_by)} (${dayWords(o.decide_by, t)})` : '—'}</dd>
            </dl>
            {o.benefits && <p className="small pre muted">{o.benefits}</p>}
            <div className="row"><span className={`pill offer-${o.status}`}>{STATUS[o.status]}</span><span className="spacer" /><Link to={`/jobs/${o.job_id}`} className="btn sm">Open</Link></div>
          </article>
        ))}
      </div>}
      {rest.length > 0 && <section className="day-sec">
        <h3 className="day-h">Decided</h3>
        <div className="card list">{rest.map(o => (
          <Link key={o.id} to={`/jobs/${o.job_id}`} className="list-row job-row">
            <span className="row-main grow"><span className="row-title"><span className="ellipsis">{o.company || o.job_title}</span></span><span className="muted small">{o.base ? `${fmtMoney(o.yearly, o.currency, true)} a year` : ''}</span></span>
            <span className={`pill offer-${o.status}`}>{STATUS[o.status]}</span>
          </Link>
        ))}</div>
      </section>}
      {advice && <Modal title="Deciding" onClose={() => setAdvice('')} size="lg" footer={<><button type="button" className="btn" onClick={() => copyText(advice).then(ok => ok && toast('Copied'))}><Icon name="copy" size={15} />Copy</button><button type="button" className="btn primary" onClick={() => setAdvice('')}>Close</button></>}>
        <div className="ai-text pre">{advice}</div>
      </Modal>}
    </div>
  )
}
