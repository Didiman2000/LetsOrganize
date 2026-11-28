-- SPDX-License-Identifier: GPL-3.0-or-later
-- Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer
--
-- Einzeln gelöschte Vorkommen einer Terminserie (EXDATE in RFC 5545).
-- Beginnzeiten in UTC im Format YYYY-MM-DDTHH:MM:SSZ, durch Leerzeichen getrennt.

ALTER TABLE events ADD COLUMN exdates TEXT NOT NULL DEFAULT '';
