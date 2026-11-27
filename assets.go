// LetsOrganize – persönlicher Organizer im Ringbuch-Stil
// Copyright (C) 2026 The LetsOrganize Authors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version. See LICENSE.

package main

import "embed"

// assets enthält Oberfläche, Sprachdateien und mitgelieferte Themen.
// Alles steckt in der Programmdatei, damit eine einzige Datei genügt.
//
//go:embed web locales themes
var assets embed.FS
