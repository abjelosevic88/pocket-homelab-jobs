// What a job search knows: the other Pocket apps, the timeline, the words
// for stages, sources, work modes and endings.
import type { ActivityKind, AppSlug, AppStateName, Job, Period, Stage, StageKind } from './types'
import { fmtMoney } from './format'

export const APP_INFO: Record<AppSlug, { name: string; icon: string; what: string }> = {
  contacts: { name: 'Pocket Contacts', icon: 'users', what: 'recruiters and companies' },
  mail: { name: 'Pocket Mail', icon: 'mail', what: 'emails' },
  calendar: { name: 'Pocket Calendar', icon: 'calendar', what: 'interviews in your calendar' },
  tasks: { name: 'Pocket Tasks', icon: 'check-square', what: 'next steps as tasks' },
  notes: { name: 'Pocket Notes', icon: 'note', what: 'notes' },
  documents: { name: 'Pocket Documents', icon: 'file-text', what: 'CVs you keep there' },
  learn: { name: 'Pocket Learn', icon: 'brain', what: 'interview flashcards' },
  weather: { name: 'Pocket Weather', icon: 'cloud', what: 'the weather for an interview' },
}

/** Which app makes each kind of linked thing. */
export const TARGET_APP: Record<string, AppSlug> = { contact: 'contacts', task: 'tasks', event: 'calendar', note: 'notes', email: 'mail', cards: 'learn' }

export const TARGET_LABEL: Record<string, string> = { contact: 'Contact', task: 'Task', event: 'In the calendar', note: 'Note', email: 'Email', cards: 'Flashcards' }

/** Short words for a state that does not work, for a greyed-out button. */
export const STATE_WORDS: Record<AppStateName, string> = {
  on: 'Connected', partly: 'Partly connected', off: 'Switched off here', link_off: 'Switched off in Pocket Core', missing: 'Not on this box',
  old: 'Needs a newer version', no_core: 'No Pocket Core', not_set_up: 'Not set up', error: 'Not reachable',
}

export const ACTIVITY: Record<ActivityKind, { icon: string; label: string }> = {
  note: { icon: 'note', label: 'Note' }, call: { icon: 'phone', label: 'Call' }, email: { icon: 'mail', label: 'Email' },
  step: { icon: 'check-circle', label: 'Step done' }, stage: { icon: 'move', label: 'Moved' }, applied: { icon: 'send', label: 'Applied' },
  interview: { icon: 'video', label: 'Interview' }, offer: { icon: 'gift', label: 'Offer' }, hired: { icon: 'trophy', label: 'Hired' },
  closed: { icon: 'x-circle', label: 'Closed' }, made: { icon: 'link', label: 'In another app' }, created: { icon: 'plus', label: 'Saved' },
}

/** Kinds a person can write on the timeline by hand. */
export const LOG_KINDS: { value: ActivityKind; label: string; icon: string }[] = [
  { value: 'note', label: 'Note', icon: 'note' }, { value: 'call', label: 'Call', icon: 'phone' }, { value: 'email', label: 'Email', icon: 'mail' },
]

export const STAGE_KINDS: { value: StageKind; label: string; help: string }[] = [
  { value: 'saved', label: 'Saved', help: 'Not applied yet' },
  { value: 'applied', label: 'Applied', help: 'Waiting to hear' },
  { value: 'active', label: 'Talking', help: 'Calls, interviews, tests' },
  { value: 'offer', label: 'Offer', help: 'An offer is on the table' },
  { value: 'hired', label: 'Hired', help: 'You took the job' },
  { value: 'closed', label: 'Closed', help: 'It ended' },
]
export const OPEN_KINDS: StageKind[] = ['saved', 'applied', 'active', 'offer']

export const SOURCES: { value: string; label: string }[] = [
  { value: 'linkedin', label: 'LinkedIn' }, { value: 'company_site', label: 'The company’s site' }, { value: 'job_board', label: 'A job board' },
  { value: 'referral', label: 'Someone recommended me' }, { value: 'recruiter', label: 'A recruiter reached out' }, { value: 'network', label: 'People I know' },
  { value: 'other', label: 'Something else' },
]
export const sourceLabel = (v: string) => SOURCES.find(s => s.value === v)?.label || (v && v !== 'not said' ? v[0].toUpperCase() + v.slice(1).replace(/_/g, ' ') : 'Not said')

export const WORK_MODES: { value: string; label: string }[] = [{ value: 'onsite', label: 'On site' }, { value: 'hybrid', label: 'Hybrid' }, { value: 'remote', label: 'Remote' }]
export const modeLabel = (v: string) => WORK_MODES.find(m => m.value === v)?.label || (v === 'not said' ? 'Not said' : '')

export const EMPLOYMENTS: { value: string; label: string }[] = [
  { value: 'full_time', label: 'Full time' }, { value: 'part_time', label: 'Part time' }, { value: 'contract', label: 'Contract' },
  { value: 'freelance', label: 'Freelance' }, { value: 'internship', label: 'Internship' },
]

export const PERIODS: { value: Period; label: string; per: string }[] = [
  { value: 'year', label: 'A year', per: '/ year' }, { value: 'month', label: 'A month', per: '/ month' }, { value: 'hour', label: 'An hour', per: '/ hour' },
]

export const END_REASONS: { value: string; label: string }[] = [
  { value: 'rejected', label: 'They said no' }, { value: 'withdrew', label: 'I withdrew' }, { value: 'no_answer', label: 'No answer' },
  { value: 'filled', label: 'The job was filled or closed' }, { value: 'other_job', label: 'I took another job' }, { value: 'other', label: 'Something else' },
]
export const endLabel = (v: string) => END_REASONS.find(r => r.value === v)?.label || 'Ended'

export const INTERVIEW_KINDS: { value: string; label: string; icon: string }[] = [
  { value: 'phone', label: 'Phone call', icon: 'phone' }, { value: 'video', label: 'Video call', icon: 'video' }, { value: 'onsite', label: 'On site', icon: 'building' },
  { value: 'technical', label: 'Technical', icon: 'code' }, { value: 'take_home', label: 'Take-home task', icon: 'home' }, { value: 'panel', label: 'Panel', icon: 'users' },
  { value: 'final', label: 'Final', icon: 'flag' }, { value: 'other', label: 'Other', icon: 'message' },
]
export const interviewKind = (v: string) => INTERVIEW_KINDS.find(k => k.value === v) || INTERVIEW_KINDS[INTERVIEW_KINDS.length - 1]

export const OUTCOMES: { value: 'good' | 'okay' | 'bad'; label: string }[] = [{ value: 'good', label: 'Went well' }, { value: 'okay', label: 'Okay' }, { value: 'bad', label: 'Not great' }]

export const CURRENCIES = ['EUR', 'BAM', 'USD', 'GBP', 'CHF', 'RSD', 'SEK', 'NOK', 'DKK', 'PLN', 'CZK', 'HUF', 'RON', 'BGN', 'TRY', 'CAD', 'AUD']

export const STAGE_COLORS = ['#868e96', '#3d5a98', '#0c8599', '#7048e8', '#ae3ec9', '#f08c00', '#2f9e44', '#e03131']

export function stageColor(s: Pick<Stage, 'color' | 'kind'> | undefined) {
  if (!s) return '#868e96'
  return s.color || (s.kind === 'hired' ? '#2f9e44' : s.kind === 'closed' ? '#e03131' : s.kind === 'offer' ? '#f08c00' : '#3d5a98')
}

/** A job in one line: “Go developer at Acme”. */
export function jobLabel(j: Pick<Job, 'title' | 'company'>) {
  if (j.title && j.company) return `${j.title} at ${j.company}`
  return j.title || j.company
}

/** The salary range in a few words (“50k–60k EUR / year”), '' when not said. */
export function salaryText(j: Pick<Job, 'salary_min' | 'salary_max' | 'currency' | 'salary_period'>, short = true) {
  const per = PERIODS.find(p => p.value === j.salary_period)?.per || ''
  const m = (c: number, cur = '') => fmtMoney(c, cur, short && j.salary_period !== 'hour').trim()
  if (j.salary_min && j.salary_max && j.salary_min !== j.salary_max) return `${m(j.salary_min)}–${m(j.salary_max, j.currency)} ${per}`
  if (j.salary_max || j.salary_min) return `${j.salary_min && !j.salary_max ? 'from ' : ''}${m(j.salary_max || j.salary_min, j.currency)} ${per}`
  return ''
}

/** “in 3 days”, “today”, “2 days ago” for a YYYY-MM-DD. */
export function dayWords(d: string, today: string) {
  if (!d) return ''
  const n = Math.round((Date.parse(d) - Date.parse(today)) / 86400000)
  if (n === 0) return 'today'
  if (n === 1) return 'tomorrow'
  if (n === -1) return 'yesterday'
  return n > 0 ? `in ${n} days` : `${-n} days ago`
}

/** The day and time of an interview in words. */
export function whenText(startsAt: string) {
  if (!startsAt) return ''
  const d = new Date(startsAt.length === 10 ? startsAt + 'T00:00' : startsAt)
  const day = d.toLocaleDateString(undefined, { weekday: 'short', day: 'numeric', month: 'short' })
  return startsAt.length > 10 ? `${day}, ${startsAt.slice(11, 16)}` : day
}
