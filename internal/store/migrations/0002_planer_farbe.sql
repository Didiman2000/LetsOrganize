-- SPDX-License-Identifier: GPL-3.0-or-later
-- Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer
--
-- Planer-Einträge bekommen eine eigene Farbe (wie die farbigen Markierungen
-- im Jahresplaner eines Ringbuchs). Erlaubte Werte prüft das Programm.

ALTER TABLE planner_entries ADD COLUMN color TEXT NOT NULL DEFAULT 'blue';
