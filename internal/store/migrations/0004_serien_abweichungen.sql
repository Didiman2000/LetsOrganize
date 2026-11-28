-- SPDX-License-Identifier: GPL-3.0-or-later
-- Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer
--
-- Abweichende Einzeltermine einer Serie, wie Google sie liefert: z. B. ein
-- Jour fixe, der einmalig auf Dienstag verschoben wurde. Der Einzeltermin
-- ist ein eigener Termin; series_remote_id verweist auf die Serie bei Google,
-- original_start auf das Vorkommen, das er ersetzt (UTC, YYYY-MM-DDTHH:MM:SSZ).
-- Beim Auflösen der Serie wird dieses Vorkommen ausgelassen.

ALTER TABLE events ADD COLUMN series_remote_id TEXT NOT NULL DEFAULT '';
ALTER TABLE events ADD COLUMN original_start TEXT NOT NULL DEFAULT '';
CREATE INDEX events_series_remote ON events(user_id, series_remote_id) WHERE series_remote_id <> '';
