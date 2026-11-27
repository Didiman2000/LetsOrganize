-- SPDX-License-Identifier: GPL-3.0-or-later
-- Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer
--
-- Grundschema von LetsOrganize.
--
-- Jede Datentabelle gehört einem Benutzer (user_id), damit derselbe Aufbau
-- später auch als Server mit mehreren Benutzern läuft.
--
-- Abgleich mit Google: remote_id und remote_etag merken sich den Stand bei
-- Google. updated_at ändert sich bei jeder lokalen Änderung, dirty = 1 heißt
-- „noch nicht übertragen“. Gelöscht wird nur weich (deleted_at), damit die
-- Löschung beim nächsten Abgleich zu Google übertragen werden kann.
--
-- Zeiten als Text im Format RFC 3339 in UTC, ganztägige Daten als YYYY-MM-DD.

CREATE TABLE users (
    id            INTEGER PRIMARY KEY,
    username      TEXT    NOT NULL UNIQUE COLLATE NOCASE,
    display_name  TEXT    NOT NULL DEFAULT '',
    password_hash TEXT    NOT NULL DEFAULT '',
    lang          TEXT    NOT NULL DEFAULT '',
    theme         TEXT    NOT NULL DEFAULT '',
    time_zone     TEXT    NOT NULL DEFAULT '',
    created_at    TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE TABLE sessions (
    token_hash TEXT    PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    expires_at TEXT    NOT NULL
);
CREATE INDEX sessions_user ON sessions(user_id);

-- Verbindung zu einem Google-Konto. Das Token wird verschlüsselt abgelegt.
CREATE TABLE google_accounts (
    id           INTEGER PRIMARY KEY,
    user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    email        TEXT    NOT NULL,
    token_enc    BLOB    NOT NULL,
    scopes       TEXT    NOT NULL DEFAULT '',
    created_at   TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    UNIQUE (user_id, email)
);

-- Merkt sich je Konto und Bereich den letzten Abgleich (Sync-Token von Google).
CREATE TABLE sync_state (
    account_id   INTEGER NOT NULL REFERENCES google_accounts(id) ON DELETE CASCADE,
    resource     TEXT    NOT NULL,          -- z. B. 'calendar:primary', 'contacts', 'tasks:@default'
    sync_token   TEXT    NOT NULL DEFAULT '',
    last_sync_at TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (account_id, resource)
);

-- Kategorien mit Farbe, nutzbar für Termine, Aufgaben und Planer.
CREATE TABLE categories (
    id      INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name    TEXT    NOT NULL,
    color   TEXT    NOT NULL DEFAULT '',
    UNIQUE (user_id, name)
);

CREATE TABLE events (
    id          INTEGER PRIMARY KEY,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title       TEXT    NOT NULL DEFAULT '',
    location    TEXT    NOT NULL DEFAULT '',
    notes       TEXT    NOT NULL DEFAULT '',
    all_day     INTEGER NOT NULL DEFAULT 0 CHECK (all_day IN (0, 1)),
    starts_at   TEXT    NOT NULL,
    ends_at     TEXT    NOT NULL,
    time_zone   TEXT    NOT NULL DEFAULT '',
    rrule       TEXT    NOT NULL DEFAULT '',  -- Wiederholung nach RFC 5545, z. B. FREQ=WEEKLY;BYDAY=MO
    reminder_minutes INTEGER,                 -- NULL = keine Erinnerung
    category_id INTEGER REFERENCES categories(id) ON DELETE SET NULL,
    account_id  INTEGER REFERENCES google_accounts(id) ON DELETE SET NULL,
    calendar_id TEXT    NOT NULL DEFAULT '',
    remote_id   TEXT    NOT NULL DEFAULT '',
    remote_etag TEXT    NOT NULL DEFAULT '',
    dirty       INTEGER NOT NULL DEFAULT 1 CHECK (dirty IN (0, 1)),
    created_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    deleted_at  TEXT,
    CHECK (ends_at >= starts_at)
);
CREATE INDEX events_user_time ON events(user_id, starts_at) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX events_remote ON events(account_id, calendar_id, remote_id) WHERE remote_id <> '';

CREATE TABLE tasks (
    id          INTEGER PRIMARY KEY,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title       TEXT    NOT NULL DEFAULT '',
    notes       TEXT    NOT NULL DEFAULT '',
    due_date    TEXT,                          -- YYYY-MM-DD oder NULL
    priority    INTEGER NOT NULL DEFAULT 0 CHECK (priority BETWEEN 0 AND 3),
    done_at     TEXT,                          -- NULL = offen
    position    INTEGER NOT NULL DEFAULT 0,
    category_id INTEGER REFERENCES categories(id) ON DELETE SET NULL,
    account_id  INTEGER REFERENCES google_accounts(id) ON DELETE SET NULL,
    list_id     TEXT    NOT NULL DEFAULT '',
    remote_id   TEXT    NOT NULL DEFAULT '',
    remote_etag TEXT    NOT NULL DEFAULT '',
    dirty       INTEGER NOT NULL DEFAULT 1 CHECK (dirty IN (0, 1)),
    created_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    deleted_at  TEXT
);
CREATE INDEX tasks_user ON tasks(user_id, done_at, due_date) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX tasks_remote ON tasks(account_id, list_id, remote_id) WHERE remote_id <> '';

CREATE TABLE contacts (
    id           INTEGER PRIMARY KEY,
    user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    given_name   TEXT    NOT NULL DEFAULT '',
    family_name  TEXT    NOT NULL DEFAULT '',
    display_name TEXT    NOT NULL DEFAULT '',
    sort_key     TEXT    NOT NULL DEFAULT '',  -- bestimmt das Register A–Z
    organization TEXT    NOT NULL DEFAULT '',
    job_title    TEXT    NOT NULL DEFAULT '',
    birthday     TEXT,                          -- YYYY-MM-DD oder --MM-DD ohne Jahr
    notes        TEXT    NOT NULL DEFAULT '',
    account_id   INTEGER REFERENCES google_accounts(id) ON DELETE SET NULL,
    remote_id    TEXT    NOT NULL DEFAULT '',  -- resourceName der People API
    remote_etag  TEXT    NOT NULL DEFAULT '',
    dirty        INTEGER NOT NULL DEFAULT 1 CHECK (dirty IN (0, 1)),
    created_at   TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at   TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    deleted_at   TEXT
);
CREATE INDEX contacts_user_sort ON contacts(user_id, sort_key) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX contacts_remote ON contacts(account_id, remote_id) WHERE remote_id <> '';

-- Telefonnummern, E-Mail-Adressen, Anschriften und Webseiten eines Kontakts.
CREATE TABLE contact_fields (
    id         INTEGER PRIMARY KEY,
    contact_id INTEGER NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
    kind       TEXT    NOT NULL CHECK (kind IN ('phone', 'email', 'address', 'url')),
    label      TEXT    NOT NULL DEFAULT '',     -- z. B. 'work', 'home', 'mobile'
    value      TEXT    NOT NULL,
    position   INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX contact_fields_contact ON contact_fields(contact_id);

CREATE TABLE notes (
    id          INTEGER PRIMARY KEY,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title       TEXT    NOT NULL DEFAULT '',
    body        TEXT    NOT NULL DEFAULT '',
    position    INTEGER NOT NULL DEFAULT 0,
    category_id INTEGER REFERENCES categories(id) ON DELETE SET NULL,
    created_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    deleted_at  TEXT
);
CREATE INDEX notes_user ON notes(user_id, position) WHERE deleted_at IS NULL;

-- Eigene Jahrestage. Geburtstage aus den Kontakten werden nicht kopiert,
-- sondern beim Anzeigen dazugenommen.
CREATE TABLE anniversaries (
    id          INTEGER PRIMARY KEY,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title       TEXT    NOT NULL,
    month       INTEGER NOT NULL CHECK (month BETWEEN 1 AND 12),
    day         INTEGER NOT NULL CHECK (day BETWEEN 1 AND 31),
    year        INTEGER,                         -- Ursprungsjahr, wenn bekannt
    category_id INTEGER REFERENCES categories(id) ON DELETE SET NULL,
    created_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    deleted_at  TEXT
);
CREATE INDEX anniversaries_user ON anniversaries(user_id, month, day) WHERE deleted_at IS NULL;

-- Zeiträume im Jahresplaner, z. B. Urlaub oder Projektphasen.
CREATE TABLE planner_entries (
    id          INTEGER PRIMARY KEY,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title       TEXT    NOT NULL DEFAULT '',
    first_day   TEXT    NOT NULL,                -- YYYY-MM-DD
    last_day    TEXT    NOT NULL,                -- YYYY-MM-DD
    category_id INTEGER REFERENCES categories(id) ON DELETE SET NULL,
    created_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    deleted_at  TEXT,
    CHECK (last_day >= first_day)
);
CREATE INDEX planner_user ON planner_entries(user_id, first_day) WHERE deleted_at IS NULL;
