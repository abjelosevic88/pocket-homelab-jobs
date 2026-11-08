// CVs, cover letters and other papers: the text (what Pocket AI reads and
// what you paste into forms) and, if you want, the file you send. One CV is
// the main one; a job can use another.
import { useCallback, useEffect, useRef, useState } from 'react'
import { api, V1 } from '../lib/api'
import { useAppState } from '../lib/data'
import { ago, fmtBytes } from '../lib/format'
import type { Doc } from '../lib/types'
import { AppOff } from '../components/Apps'
import { Icon } from '../components/icons'
import { Confirm, Empty, Field, Loading, Modal, Segmented, Spinner, copyText, errMsg, useToast } from '../components/ui'

const KINDS: { value: Doc['kind']; label: string }[] = [{ value: 'cv', label: 'CV' }, { value: 'letter', label: 'Cover letter' }, { value: 'other', label: 'Other' }]

export default function Docs() {
  const { toast } = useToast()
  const [list, setList] = useState<Doc[] | null>(null)
  const [cur, setCur] = useState<Doc | null>(null)
  const [adding, setAdding] = useState(false)
  const [fromDocs, setFromDocs] = useState(false)
  const load = useCallback(async () => { try { setList(await api.get<Doc[]>(`${V1}/docs?full=1`)) } catch { setList([]) } }, [])
  useEffect(() => { load() }, [load])
  const appState = useAppState()
  const documents = appState('documents', 'documents:read')
  const open = async (d: Doc) => { try { setCur(await api.get<Doc>(`${V1}/docs/${d.id}`)) } catch (e) { toast(errMsg(e), 'err') } }
  if (!list) return <Loading />
  return (
    <div className="page stack loose">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">CVs & letters</h1><p className="muted">Pocket AI reads your CV to check how well you fit a job, write cover letters and prepare you for interviews.</p></div>
        <button type="button" className="btn" onClick={() => setFromDocs(true)} disabled={documents.loading} title={documents.ok ? 'Bring a CV you keep in Pocket Documents' : 'Pocket Documents is not connected'}><Icon name="file-text" size={15} /><span className="hide-sm">From Pocket Documents</span></button>
        <button type="button" className="btn primary" onClick={() => setAdding(true)}><Icon name="plus" size={15} />Add</button>
      </div>
      {!list.length && <Empty title="Add your CV" icon="file-text">Paste its text (and, if you like, add the PDF you send). With it, Pocket AI can tell you how well you fit each job and write cover letters from it.</Empty>}
      {KINDS.map(k => {
        const items = list.filter(d => d.kind === k.value)
        if (!items.length) return null
        return (
          <section key={k.value} className="day-sec">
            <h3 className="day-h">{k.value === 'cv' ? 'CVs' : k.value === 'letter' ? 'Cover letters' : 'Other papers'}<span className="count">{items.length}</span></h3>
            <div className="doc-grid">
              {items.map(d => (
                <button key={d.id} type="button" className="card doc-card" onClick={() => open(d)}>
                  <div className="row tight"><Icon name="file-text" size={16} /><b className="grow ellipsis">{d.name}</b>{d.is_default && <span className="pill st-done">Main</span>}</div>
                  <p className="doc-peek small muted">{d.body ? d.body.slice(0, 220) : 'No text yet — add it so Pocket AI can read it.'}</p>
                  <div className="muted tiny">{[d.file_name && `${d.file_name} · ${fmtBytes(d.file_size)}`, d.used && `sent to ${d.used} ${d.used === 1 ? 'job' : 'jobs'}`, `changed ${ago(d.updated_at)}`].filter(Boolean).join(' · ')}</div>
                </button>
              ))}
            </div>
          </section>
        )
      })}
      {(adding || cur) && <DocSheet doc={cur} onClose={() => { setAdding(false); setCur(null) }} onDone={() => { setAdding(false); setCur(null); load() }} />}
      {fromDocs && <FromDocuments ok={documents.ok} app={documents.app} onClose={() => setFromDocs(false)} onDone={() => { setFromDocs(false); load() }} />}
    </div>
  )
}

function DocSheet({ doc, onClose, onDone }: { doc: Doc | null; onClose: () => void; onDone: () => void }) {
  const { toast } = useToast()
  const [f, setF] = useState({ kind: doc?.kind || 'cv' as Doc['kind'], name: doc?.name || '', body: doc?.body || '', is_default: !!doc?.is_default })
  const [file, setFile] = useState<{ name: string; size: number; type: string } | null>(doc?.file_name ? { name: doc.file_name, size: doc.file_size, type: doc.file_type } : null)
  const [busy, setBusy] = useState(false)
  const [del, setDel] = useState(false)
  const input = useRef<HTMLInputElement>(null)
  const [pending, setPending] = useState<File | null>(null)
  const save = async () => {
    setBusy(true)
    try {
      const d = doc ? await api.put<Doc>(`${V1}/docs/${doc.id}`, f) : await api.post<Doc>(`${V1}/docs`, f)
      if (pending) {
        const res = await fetch(`${V1}/docs/${d.id}/file?name=${encodeURIComponent(pending.name)}`, { method: 'PUT', body: pending, headers: { 'Content-Type': pending.type || 'application/octet-stream' }, credentials: 'same-origin' })
        if (!res.ok) throw new Error((await res.json().catch(() => ({}))).error || `Upload failed (${res.status})`)
      }
      toast('Saved')
      onDone()
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  const pick = (fl: File | undefined) => {
    if (!fl) return
    if (fl.size > 15 << 20) { toast('A file is up to 15 MB', 'err'); return }
    setPending(fl)
    setFile({ name: fl.name, size: fl.size, type: fl.type })
    if (!f.name) setF(x => ({ ...x, name: fl.name.replace(/\.[^.]+$/, '') }))
    if (!f.body && /^text\/|\.(md|txt)$/i.test(fl.type || fl.name)) fl.text().then(txt => setF(x => ({ ...x, body: x.body || txt })))
  }
  const removeFile = async () => {
    if (doc?.file_name) await api.del(`${V1}/docs/${doc.id}/file`).catch(() => {})
    setFile(null); setPending(null)
  }
  const remove = async () => { try { await api.del(`${V1}/docs/${doc!.id}`); toast('Deleted'); onDone() } catch (e) { toast(errMsg(e), 'err') } }
  return (
    <Modal title={doc ? doc.name : 'Add a CV or a letter'} onClose={onClose} size="lg" footer={<>
      {doc && <button type="button" className="btn danger-ghost" onClick={() => setDel(true)}><Icon name="trash" size={15} /><span className="hide-sm">Delete</span></button>}
      {f.body && <button type="button" className="btn" onClick={() => copyText(f.body).then(ok => ok && toast('Copied'))}><Icon name="copy" size={15} /><span className="hide-sm">Copy the text</span></button>}
      <span className="spacer" />
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" onClick={save} disabled={busy || !(f.name || f.body || pending)}>{busy ? <Spinner /> : <Icon name="check" size={15} />}Save</button>
    </>}>
      <div className="stack">
        <div className="two-col tight">
          <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} placeholder="CV — one page, payments" autoFocus={!doc} /></Field>
          <Field label="What it is" group><Segmented size="sm" label="What it is" value={f.kind} onChange={v => setF({ ...f, kind: v })} options={KINDS} /></Field>
        </div>
        {f.kind === 'cv' && !doc?.is_default && <label className="check-row"><input type="checkbox" checked={f.is_default} onChange={e => setF({ ...f, is_default: e.target.checked })} />Make it my main CV (jobs use it unless you pick another)</label>}
        <Field label="The text" help="Paste it from your CV. This is what Pocket AI reads; formatting does not matter."><textarea rows={14} value={f.body} onChange={e => setF({ ...f, body: e.target.value })} className="mono-ish" /></Field>
        <div className="file-row">
          <Icon name="upload" size={16} />
          {file ? <span className="grow ellipsis">{doc?.file_name && !pending ? <a href={`${V1}/docs/${doc.id}/file`} download>{file.name}</a> : file.name} <span className="muted small">· {fmtBytes(file.size)}</span></span> : <span className="grow muted">The file you send (PDF, Word…), up to 15 MB — optional</span>}
          {file && <button type="button" className="btn sm ghost" onClick={removeFile}>Remove</button>}
          <button type="button" className="btn sm" onClick={() => input.current?.click()}>{file ? 'Replace' : 'Choose a file'}</button>
          <input ref={input} type="file" hidden onChange={e => pick(e.target.files?.[0])} />
        </div>
        {doc?.doc_url && <p className="muted small"><Icon name="file-text" size={12} className="inline-ico" /> From <a href={doc.doc_url} target="_blank" rel="noreferrer">Pocket Documents</a>.</p>}
      </div>
      {del && <Confirm title="Delete it?" message={<>“{doc?.name}” goes. Jobs that used it keep everything else.</>} confirmLabel="Delete" onConfirm={remove} onCancel={() => setDel(false)} />}
    </Modal>
  )
}

type DocHit = { id: string; title: string; created: string; mime: string; size: number; name: string; url: string }

function FromDocuments({ ok, app, onClose, onDone }: { ok: boolean; app?: import('../lib/types').AppState; onClose: () => void; onDone: () => void }) {
  const { toast } = useToast()
  const [q, setQ] = useState('cv')
  const [res, setRes] = useState<{ state: string; detail?: string; items: DocHit[] } | null>(null)
  const [kind, setKind] = useState<Doc['kind']>('cv')
  const [busy, setBusy] = useState('')
  const search = useCallback(async (v: string) => {
    try { setRes(await api.get(`${V1}/docs/from-documents?q=${encodeURIComponent(v)}`)) } catch (e) { setRes({ state: 'error', detail: errMsg(e), items: [] }) }
  }, [])
  useEffect(() => { if (ok) search(q) }, [ok]) // eslint-disable-line react-hooks/exhaustive-deps
  const take = async (h: DocHit) => {
    setBusy(h.id)
    try {
      const r = await api.post<{ doc: Doc; warning?: string }>(`${V1}/docs/from-documents`, { id: h.id, kind, name: h.title })
      toast(r.warning || `Brought “${r.doc.name}”`, r.warning ? 'err' : undefined)
      onDone()
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy('') }
  }
  return (
    <Modal title="From Pocket Documents" onClose={onClose} footer={<button type="button" className="btn" onClick={onClose}>Close</button>}>
      {!ok ? <AppOff slug="documents" app={app}>Keep your CV in Pocket Documents, and bring it here (with the text Documents read from it) in one tap.</AppOff> : <div className="stack">
        <form className="row tight" onSubmit={e => { e.preventDefault(); search(q) }}>
          <div className="search-in grow"><Icon name="search" size={15} /><input value={q} onChange={e => setQ(e.target.value)} placeholder="cv, resume, letter…" aria-label="Search Pocket Documents" /></div>
          <button type="submit" className="btn">Search</button>
        </form>
        <Segmented size="sm" label="Bring it as" value={kind} onChange={setKind} options={KINDS} />
        {!res ? <Loading /> : res.state !== 'ok' ? <p className="muted small">{res.detail}</p> : !res.items.length ? <p className="muted small">Nothing called “{q}” in Pocket Documents.</p> : (
          <div className="card list">
            {res.items.map(h => (
              <div key={h.id} className="list-row">
                <span className="list-ico"><Icon name="file-text" size={16} /></span>
                <span className="row-main grow"><span className="row-title"><span className="ellipsis">{h.title}</span></span><span className="muted small">{[h.created, h.size ? fmtBytes(h.size) : ''].filter(Boolean).join(' · ')}</span></span>
                <button type="button" className="btn sm" onClick={() => take(h)} disabled={!!busy}>{busy === h.id ? <Spinner /> : <Icon name="download" size={14} />}Bring</button>
              </div>
            ))}
          </div>
        )}
      </div>}
    </Modal>
  )
}
