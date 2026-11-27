// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

package store

// SQLite-Treiber in reinem Go: keine C-Bibliotheken nötig, daher lässt sich
// LetsOrganize ohne weitere Werkzeuge für Windows, Linux und macOS bauen.
import _ "modernc.org/sqlite"

const driverName = "sqlite"
