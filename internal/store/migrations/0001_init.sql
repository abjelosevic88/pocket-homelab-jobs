CREATE TABLE users (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  email         TEXT NOT NULL UNIQUE,
  name          TEXT NOT NULL DEFAULT '',
  password_hash TEXT NOT NULL DEFAULT '',
  role          TEXT NOT NULL DEFAULT 'member',
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT
);

CREATE TABLE sessions (
  id         TEXT PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE api_tokens (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL DEFAULT '',
  token_hash   TEXT NOT NULL UNIQUE,
  prefix       TEXT NOT NULL DEFAULT '',
  last_used_at TEXT,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- Box-wide settings (title, setup flag); everything else is personal.
CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT ''
);

CREATE TABLE user_settings (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  key     TEXT NOT NULL,
  value   TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (user_id, key)
);

CREATE TABLE cache (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL DEFAULT '',
  fetched_at TEXT NOT NULL
);


-- ---------- Pocket Jobs: everything below is per person ----------

-- The steps of a person's job search, in order. kind says what a job in
-- this step means: saved (not applied yet), applied (waiting to hear),
-- active (talking: calls, interviews), offer, hired, closed (it ended).
CREATE TABLE stages (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name       TEXT NOT NULL,
  kind       TEXT NOT NULL DEFAULT 'active',
  position   INTEGER NOT NULL DEFAULT 0,
  color      TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX stages_user ON stages(user_id, position);

-- CVs, cover letters and other papers a person sends, as text (what the AI
-- reads) and, if they want, the file itself.
CREATE TABLE docs (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind       TEXT NOT NULL DEFAULT 'cv',                  -- cv | letter | other
  name       TEXT NOT NULL DEFAULT '',
  body       TEXT NOT NULL DEFAULT '',
  file       BLOB,
  file_name  TEXT NOT NULL DEFAULT '',
  file_type  TEXT NOT NULL DEFAULT '',
  file_size  INTEGER NOT NULL DEFAULT 0,
  doc_ref    TEXT NOT NULL DEFAULT '',                    -- Pocket Documents id it came from
  doc_url    TEXT NOT NULL DEFAULT '',
  is_default INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX docs_user ON docs(user_id, kind, updated_at);

-- A job: an opening a person is thinking about, applied for, interviewing
-- for… until hired or closed. The contact (a recruiter, the hiring
-- manager) and the company are text here so the app works without Pocket
-- Contacts; contact_ref / company_ref point at the cards there.
CREATE TABLE jobs (
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id          INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  title            TEXT NOT NULL DEFAULT '',              -- the role
  company          TEXT NOT NULL DEFAULT '',
  company_ref      TEXT NOT NULL DEFAULT '',
  stage_id         INTEGER NOT NULL REFERENCES stages(id),
  status           TEXT NOT NULL DEFAULT 'open',          -- open | hired | closed
  reached          INTEGER NOT NULL DEFAULT 0,            -- furthest kind: 0 saved, 1 applied, 2 active, 3 offer, 4 hired
  location         TEXT NOT NULL DEFAULT '',
  work_mode        TEXT NOT NULL DEFAULT '',              -- onsite | hybrid | remote
  employment       TEXT NOT NULL DEFAULT '',              -- full_time | part_time | contract | freelance | internship
  url              TEXT NOT NULL DEFAULT '',              -- the posting
  posting          TEXT NOT NULL DEFAULT '',              -- the posting's text
  salary_min       INTEGER NOT NULL DEFAULT 0,            -- in cents
  salary_max       INTEGER NOT NULL DEFAULT 0,
  currency         TEXT NOT NULL DEFAULT 'EUR',
  salary_period    TEXT NOT NULL DEFAULT 'year',          -- year | month | hour
  source           TEXT NOT NULL DEFAULT '',
  excitement       INTEGER NOT NULL DEFAULT 0,            -- 0–5
  apply_by         TEXT NOT NULL DEFAULT '',              -- YYYY-MM-DD
  applied_on       TEXT NOT NULL DEFAULT '',              -- YYYY-MM-DD
  responded_at     TEXT NOT NULL DEFAULT '',              -- when they first answered
  contact_name     TEXT NOT NULL DEFAULT '',
  contact_email    TEXT NOT NULL DEFAULT '',
  contact_phone    TEXT NOT NULL DEFAULT '',
  contact_ref      TEXT NOT NULL DEFAULT '',
  notes            TEXT NOT NULL DEFAULT '',
  cover_letter     TEXT NOT NULL DEFAULT '',
  cv_id            INTEGER REFERENCES docs(id) ON DELETE SET NULL,
  fit_score        INTEGER NOT NULL DEFAULT -1,           -- 0–100 from the AI, -1 = not checked
  fit_note         TEXT NOT NULL DEFAULT '',
  next_action      TEXT NOT NULL DEFAULT '',
  next_due         TEXT NOT NULL DEFAULT '',
  next_task_ref    TEXT NOT NULL DEFAULT '',
  tags             TEXT NOT NULL DEFAULT '[]',
  starred          INTEGER NOT NULL DEFAULT 0,
  position         REAL NOT NULL DEFAULT 0,
  end_reason       TEXT NOT NULL DEFAULT '',              -- rejected | withdrew | no_answer | filled | other_job | other
  outcome_note     TEXT NOT NULL DEFAULT '',
  closed_at        TEXT NOT NULL DEFAULT '',
  stage_changed_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  last_activity_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  source_app       TEXT NOT NULL DEFAULT '',
  source_ref       TEXT NOT NULL DEFAULT '',
  client_key       TEXT NOT NULL DEFAULT '',
  created_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX jobs_user ON jobs(user_id, status, stage_id, position);
CREATE INDEX jobs_due ON jobs(user_id, status, next_due);
CREATE INDEX jobs_email ON jobs(user_id, contact_email);
CREATE INDEX jobs_company ON jobs(user_id, company);
CREATE INDEX jobs_applied ON jobs(user_id, applied_on);
CREATE UNIQUE INDEX jobs_client ON jobs(user_id, client_key) WHERE client_key <> '';
CREATE UNIQUE INDEX jobs_source ON jobs(user_id, source_ref) WHERE source_ref <> '';

-- Interviews: each call, video, visit or test on the way.
CREATE TABLE interviews (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  job_id      INTEGER NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
  kind        TEXT NOT NULL DEFAULT 'video',               -- phone | video | onsite | technical | take_home | panel | final | other
  starts_at   TEXT NOT NULL DEFAULT '',                    -- local YYYY-MM-DDTHH:MM (or a day)
  minutes     INTEGER NOT NULL DEFAULT 60,
  location    TEXT NOT NULL DEFAULT '',                    -- an address or a call link
  with_people TEXT NOT NULL DEFAULT '',
  prep        TEXT NOT NULL DEFAULT '',
  questions   TEXT NOT NULL DEFAULT '[]',                  -- [{q, a}]
  outcome     TEXT NOT NULL DEFAULT '',                    -- good | okay | bad
  feedback    TEXT NOT NULL DEFAULT '',
  thanks_sent INTEGER NOT NULL DEFAULT 0,
  event_ref   TEXT NOT NULL DEFAULT '',                    -- Pocket Calendar event
  event_url   TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX interviews_user ON interviews(user_id, starts_at);
CREATE INDEX interviews_job ON interviews(job_id, starts_at);

-- An offer for a job (one per job), to weigh against the others.
CREATE TABLE offers (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  job_id     INTEGER NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
  base       INTEGER NOT NULL DEFAULT 0,                   -- in cents, per period
  currency   TEXT NOT NULL DEFAULT 'EUR',
  period     TEXT NOT NULL DEFAULT 'year',
  bonus      INTEGER NOT NULL DEFAULT 0,                   -- a year, in cents
  equity     TEXT NOT NULL DEFAULT '',
  pto_days   INTEGER NOT NULL DEFAULT 0,
  benefits   TEXT NOT NULL DEFAULT '',
  start_date TEXT NOT NULL DEFAULT '',
  decide_by  TEXT NOT NULL DEFAULT '',
  status     TEXT NOT NULL DEFAULT 'open',                 -- open | negotiating | accepted | declined
  notes      TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE UNIQUE INDEX offers_job ON offers(user_id, job_id);

-- What happened on a job: notes, calls, emails, steps done, moves between
-- stages, interviews, things made in the other apps.
CREATE TABLE activities (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  job_id     INTEGER NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
  kind       TEXT NOT NULL DEFAULT 'note',               -- note | call | email | step | stage | applied | interview | offer | hired | closed | made | created
  title      TEXT NOT NULL DEFAULT '',
  body       TEXT NOT NULL DEFAULT '',
  at         TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  app        TEXT NOT NULL DEFAULT '',
  url        TEXT NOT NULL DEFAULT '',
  source_app TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX activities_job ON activities(job_id, at);
CREATE INDEX activities_user ON activities(user_id, at);

-- What a job has in the other Pocket apps (a contact card, a task, a
-- calendar event, a note, flashcards…), with a link to it.
CREATE TABLE made (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  job_id     INTEGER NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
  target     TEXT NOT NULL,                              -- contact | task | event | note | email | cards
  app        TEXT NOT NULL DEFAULT '',
  ref        TEXT NOT NULL DEFAULT '',
  title      TEXT NOT NULL DEFAULT '',
  url        TEXT NOT NULL DEFAULT '',
  state      TEXT NOT NULL DEFAULT '',
  error      TEXT NOT NULL DEFAULT '',
  checked_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX made_user ON made(user_id, job_id);
