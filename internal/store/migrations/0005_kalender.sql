-- SPDX-License-Identifier: GPL-3.0-or-later
-- Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer
--
-- Mehrere Kalender eines Google-Kontos (Hauptkalender, eigene, freigegebene,
-- Feiertage). remote_id ist die Kalender-ID bei Google; der Hauptkalender
-- heißt hier wie in events.calendar_id immer 'primary'.
-- enabled = 0: Kalender wird weder abgeglichen noch angezeigt.
-- access_role wie bei Google: owner, writer, reader, freeBusyReader.

CREATE TABLE calendars (
    id          INTEGER PRIMARY KEY,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_id  INTEGER NOT NULL REFERENCES google_accounts(id) ON DELETE CASCADE,
    remote_id   TEXT    NOT NULL,
    name        TEXT    NOT NULL DEFAULT '',
    color       TEXT    NOT NULL DEFAULT '',
    access_role TEXT    NOT NULL DEFAULT 'reader',
    is_primary  INTEGER NOT NULL DEFAULT 0 CHECK (is_primary IN (0, 1)),
    enabled     INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    position    INTEGER NOT NULL DEFAULT 0,
    UNIQUE (account_id, remote_id)
);
CREATE INDEX calendars_user ON calendars(user_id);
