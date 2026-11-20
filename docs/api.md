# API

JSON over HTTP at `/api/v1`. Sign in with the session cookie (the web app), or send a personal token from Settings → API & shortcuts:
`Authorization: Bearer pjb_…`. Everything is per person: another person's ids answer 404.

Salaries are kept in **cents** (`salary_min`, `salary_max`, an offer's `base` and `bonus`); every job also carries `min_amount` / `max_amount`
in the currency's units, and you send those (`{"min_amount": 50000}`). Days are `YYYY-MM-DD`; an interview's `starts_at` is local `YYYY-MM-DDTHH:MM`.

## Jobs

| | |
|---|---|
| `GET /jobs` | `status` (open, hired, closed, ended), `kind` (saved, applied, active, offer, hired, closed), `stage`, `q`, `company`, `email`, `due` (overdue, today, week, none), `work_mode`, `source`, `tag`, `starred`, `sort` (stage, due, excitement, fit, salary, applied, updated, created), `limit`, `offset` → `{items, total}` |
| `POST /jobs` | `{title, company, url, location, work_mode, employment, min_amount, max_amount, currency, salary_period, source, excitement, apply_by, stage, contact_name, contact_email, notes, posting, next_action, next_due, tags, cv_id, source_ref}`. `stage` is a stage's name or a kind; `source_ref` makes it never added twice; `?once=1` returns the job already saved with the same `url` |
| `GET /jobs/{id}` | `{job, activities, interviews, offer, made, url}` |
| `PUT /jobs/{id}` | any of the fields above |
| `DELETE /jobs/{id}` | |
| `POST /jobs/{id}/move` | `{stage_id, before, after, end_reason, note}` — `end_reason` for a closed stage: rejected, withdrew, no_answer, filled, other_job, other |
| `POST /jobs/{id}/step/done` | `{kind, note, next_action, next_due}` |
| `GET/POST /jobs/{id}/activities` | the timeline; `{kind: note\|call\|email\|step, title, body, at}` |
| `POST /jobs/{id}/make` | `{target: contact\|task\|note\|email, …}` in another Pocket app |
| `POST /jobs/{id}/ai` | `{want: fit\|letter\|tailor\|prep\|email\|next\|summary\|research, kind, hint, interview_id}` |
| `GET /jobs/{id}/elsewhere` | the emails and calendar events about the job, from Pocket Mail and Calendar |
| `POST /postings/read` | `{url}` or `{text}` → the fields of a job, not saved |

## Interviews, offers, papers

| | |
|---|---|
| `GET /interviews` | `when` (upcoming, past), `job`, `from`, `to` |
| `POST /jobs/{id}/interviews` | `{kind, date, time, minutes, location, with_people, prep, questions: [{q, a}], calendar}` |
| `PUT /interviews/{id}` | the same, plus `outcome` (good, okay, bad), `feedback`, `thanks_sent` |
| `POST /interviews/{id}/calendar` · `/cards` · `/note` | to Pocket Calendar, Learn (flashcards), Notes |
| `GET /interviews/{id}/interview.ics` · `/clashes` · `/weather` | |
| `GET /offers` · `PUT /jobs/{id}/offer` · `DELETE /jobs/{id}/offer` | `{base_amount, bonus_amount, currency, period, pto_days, equity, benefits, start_date, decide_by, status}` |
| `POST /offers/ai` | help deciding between the open offers |
| `GET/POST /docs` · `GET/PUT/DELETE /docs/{id}` | CVs and letters: `{kind: cv\|letter\|other, name, body, is_default}` |
| `PUT /docs/{id}/file?name=cv.pdf` | the body is the file (up to 15 MB); `GET` downloads it |
| `GET /docs/from-documents?q=` · `POST /docs/from-documents {id, kind}` | from Pocket Documents |

## Overviews

`GET /today`, `GET /summary` (for a Homepage widget), `GET /companies`, `GET /reports?weeks=12`, `GET /search?q=`, `GET /lookup?email=|company=`
(for a contact's page in another app), `GET /day/YYYY-MM-DD` (for Pocket Day), `GET /stages` (and `POST/PUT/DELETE` to change them).

## Data

`GET /export.json`, `GET /export.csv`, `POST /import` (a Pocket Jobs export or a CSV — multipart `file` or the raw body), `GET /backup` (admins).

Pocket Move can copy your job search out and bring jobs in: `GET /pocket/portable` describes the exports (link scope `move:export`) and the imports (`move:import`). Imports only add; a second run of the same file adds nothing.
