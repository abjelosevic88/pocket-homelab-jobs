export interface User { id: number; email: string; name: string; role: 'admin' | 'member'; created_at: string }
export interface APIToken { id: number; user_id: number; name: string; prefix: string; last_used_at: string | null; created_at: string }

export type ConnMode = 'core' | 'direct' | 'off'
export type ConnVia = '' | 'core' | 'direct'
export type AppSlug = 'contacts' | 'mail' | 'calendar' | 'tasks' | 'notes' | 'documents' | 'learn' | 'weather'

type ConnFields = { [K in AppSlug as `${K}_mode`]: ConnMode } & { [K in AppSlug as `${K}_via`]: ConnVia } & { [K in AppSlug as `${K}_detail` | `${K}_url` | `${K}_token` | `${K}_public_url` | `${K}_open_url`]: string } & { [K in AppSlug as `${K}_configured`]: boolean }

export type Period = 'year' | 'month' | 'hour'

export type Settings = ConnFields & {
  core_configured: boolean
  core_registered?: boolean
  title: string; date_format: string; time_format: string; week_start: number
  currency: string; period: Period; weekly_goal: number; stale_days: number; action_days: number; home: string
  task_mirror: boolean; interview_minutes: number; calendar_sync: boolean; learn_deck: string; mail_from: string
  ai_help: boolean; about_me: string
  notify_digest: boolean; notify_hour: number
  notify_url: string; notify_format: string; notify_configured: boolean
  ai_provider: 'auto' | 'pocket' | 'openai'; ai_pocket_model: string; openai_api_key: string; openai_model: string; openai_base_url: string
  setup_complete: boolean; base_url: string; demo: boolean
}

export type StageKind = 'saved' | 'applied' | 'active' | 'offer' | 'hired' | 'closed'
export type Status = 'open' | 'hired' | 'closed'

export interface Stage { id: number; name: string; kind: StageKind; position: number; color: string; created_at: string; jobs: number }

export interface Job {
  id: number; title: string; company: string; company_ref: string; stage_id: number; status: Status; reached: number
  location: string; work_mode: string; employment: string; url: string; posting: string
  salary_min: number; salary_max: number; min_amount: number; max_amount: number; currency: string; salary_period: Period
  source: string; excitement: number; apply_by: string; applied_on: string; responded_at: string
  contact_name: string; contact_email: string; contact_phone: string; contact_ref: string
  notes: string; cover_letter: string; cv_id: number; cv_name?: string; fit_score: number; fit_note: string
  next_action: string; next_due: string; next_task_ref: string; tags: string[]; starred: boolean; position: number
  end_reason: string; outcome_note: string; closed_at: string; stage_changed_at: string; last_activity_at: string
  source_app: string; source_ref: string; created_at: string; updated_at: string
  stage_name?: string; stage_kind?: StageKind; stage_color?: string
  interviews: number; next_interview?: string; has_offer: boolean; overdue: boolean; stale: boolean
  made_count: number; activities: number; days_in_stage: number
}

export type ActivityKind = 'note' | 'call' | 'email' | 'step' | 'stage' | 'applied' | 'interview' | 'offer' | 'hired' | 'closed' | 'made' | 'created'

export interface Activity { id: number; job_id: number; kind: ActivityKind; title: string; body: string; at: string; app: string; url: string; source_app: string; created_at: string }

export interface Made { id: number; job_id: number; target: string; app: string; ref: string; title: string; url: string; state: string; error?: string; checked_at: string; created_at: string }

export interface QA { q: string; a: string }

export interface Interview {
  id: number; job_id: number; kind: string; starts_at: string; minutes: number; location: string; with_people: string; prep: string
  questions: QA[]; outcome: '' | 'good' | 'okay' | 'bad'; feedback: string; thanks_sent: boolean; event_ref: string; event_url: string
  created_at: string; updated_at: string; job_title?: string; company?: string; job_status?: Status; stage_color?: string; past: boolean
}

export interface Offer {
  id: number; job_id: number; base: number; base_amount: number; currency: string; period: Period; bonus: number; bonus_amount: number
  equity: string; pto_days: number; benefits: string; start_date: string; decide_by: string; status: 'open' | 'negotiating' | 'accepted' | 'declined'
  notes: string; created_at: string; updated_at: string; yearly: number
  job_title?: string; company?: string; location?: string; work_mode?: string; excitement: number
}

export interface Doc {
  id: number; kind: 'cv' | 'letter' | 'other'; name: string; body: string; file_name: string; file_type: string; file_size: number
  doc_ref: string; doc_url: string; is_default: boolean; created_at: string; updated_at: string; used: number
}

export interface JobView { job: Job; activities: Activity[]; made: Made[]; interviews: Interview[]; offer: Offer | null; url: string }

export type AppStateName = 'on' | 'partly' | 'off' | 'link_off' | 'missing' | 'old' | 'no_core' | 'not_set_up' | 'error'

export interface AppState { slug: AppSlug; name: string; reason: string; mode: ConnMode; via: ConnVia; state: AppStateName; detail: string; fix: string; on: string[]; open_url: string }

export interface ElseItem { kind: 'email' | 'meeting'; title: string; body?: string; at: string; url?: string; state?: string; out?: boolean; ref?: string }
export interface ElseSection { app: AppSlug; name: string; state: 'ok' | 'off' | 'error' | 'none'; detail?: string; fix?: string; items: ElseItem[]; open_url?: string }

export interface Summary {
  open: number; saved: number; waiting: number; talking: number; offers: number; hired: number; closed: number
  weekly_goal: number; applied_week: number; applied_month: number; applied_total: number; response_rate: number; interview_rate: number
  interviews_upcoming: number; next_interview: Interview | null; due_today: number; overdue: number; follow_up: number; apply_soon: number
  week_start: string; url: string
}

export interface TodayView {
  lists: Record<'overdue' | 'today' | 'week' | 'follow_up' | 'apply' | 'no_step' | 'recent', Job[]>
  interviews: Interview[]; offers: Offer[]; week: { date: string; applied: number }[]; summary: Summary; date: string
}

export interface Company {
  key: string; name: string; company_ref: string; jobs: number; open: number; hired: number; closed: number; interviews: number
  last_at: string; people: string[]; emails: string[]; roles: string[]; job_ids: number[]
}

export interface CountRow { name: string; count: number; answered: number; interviews: number; offers: number }
export interface WeekRow { week: string; applied: number; interviews: number; answers: number }
export interface Reports { weeks: WeekRow[]; funnel: CountRow[]; sources: CountRow[]; work_modes: CountRow[]; end_reasons: CountRow[]; days_to_answer: number; summary: Summary }

export interface PickedContact { id: string; kind: string; name: string; company: string; title: string; email: string; phone: string; initials: string; color: string }

export interface Posting {
  url: string; title: string; company: string; location: string; work_mode: string; employment: string
  min_amount: number; max_amount: number; currency: string; salary_period: string; apply_by: string; posting: string; source: string; structured: boolean
}
