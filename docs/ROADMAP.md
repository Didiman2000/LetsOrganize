# LetsOrganize – Roadmap Version 1

Stand: 27.11.2026

## Ziel

Persönlicher Organizer im Browser mit der Optik eines hochwertigen Leder-Ringbuchs mit farbigen Registern, angelehnt an das klassische Organizer-Layout, aber mit Abgleich zu Google Kalender, Google Kontakte und Google Tasks. Zunächst für den eigenen Gebrauch, als Open Source unter der GPL 3.0.

## Entscheidungen

- Browser-Anwendung, Backend in Go: eine Programmdatei, lokal startbar, später im Container betreibbar
- Eigenes Design ohne Original-Grafiken und ohne den Namen „Lotus“
- Oberfläche vollständig über **Themen** gestaltbar (Farben, Einband, Papier je Einlage, Schriften, Registerformen); Themen als Datei austauschbar
- **Mehrsprachig** von Anfang an: alle Texte in Sprachdateien, Start mit Deutsch und Englisch, Datums- und Zahlenformate je Sprache; Beschriftungen kommen aus der Sprachdatei, nicht aus dem Thema
- **Rechts-nach-links-Sprachen** (z. B. Arabisch, Hebräisch): Layout nur mit logischen CSS-Eigenschaften, Ringbuch gespiegelt
- Google-Zugang über eigenes OAuth-Projekt; bei Open Source legt jeder Nutzer seinen eigenen Zugang an
- Import der Altdaten aus Organizer 6/6.1 per CSV-/Text-Export, kein Einlesen des Binärformats
- Vorbereitung auf Serverbetrieb: Anmeldung, Daten je Nutzer getrennt, Google-Tokens verschlüsselt
- Lizenz: GPL 3.0 oder später

## Register in Version 1

1. Kalender – Tages-, Wochen- (Doppelseite) und Monatsansicht
2. Aufgaben – Abgleich mit Google Tasks
3. Adressen – Abgleich mit Google Kontakte, Register A–Z
4. Notizen – lokal gespeichert
5. Planer – Jahresübersicht mit farbigen Markierungen
6. Jahrestage – automatisch aus Geburtstagen der Kontakte, plus eigene

Später: Anrufe, Gmail-Anbindung, Verknüpfungen zwischen Einträgen, Mehrbenutzer-Server.

## Phasen

### Phase 0 – Vorbereitung
- [x] Name (LetsOrganize), Repository, Lizenz (GPL 3.0)
- [ ] Alte Organizer-Installation starten (ggf. in einer VM)
- [ ] Probeexport aller Bereiche als CSV/Text erstellen und Felder dokumentieren
- [ ] Google-Cloud-Projekt anlegen, Calendar-, People- und Tasks-API aktivieren, OAuth-Client erstellen

### Phase 1 – Grundgerüst
- [x] Go-Projekt mit Webserver, eingebettete Oberfläche, Sicherheits-Header (CSP)
- [x] Mehrsprachigkeit: Sprachdateien (de, en, ar), Sprachwahl, Wochenbeginn je Sprache, Rückfall auf Englisch, Test auf Vollständigkeit
- [x] Rechts-nach-links: logische CSS-Eigenschaften, gespiegeltes Ringbuch, Test mit Arabisch
- [x] Themen: zwei mitgelieferte Themen, eigene Themen im Datenordner, strenge Prüfung der Werte
- [x] Ringbuch-Oberfläche: Einband, Ringe, Register, Wochenansicht, Jahresplaner (ohne Daten)
- [ ] SQLite-Datenbank mit Migrationen (Treiber `modernc.org/sqlite`, reines Go)
- [ ] Datenmodell: Termine, Aufgaben, Kontakte, Notizen, Jahrestage, Planer-Einträge
- [ ] Anmeldung (einfach, aber serverfähig) und Nutzer-Trennung im Datenmodell
- [ ] Build für Windows mit Programmsymbol, Dockerfile

### Phase 2 – Ringbuch-Oberfläche ausbauen
- [ ] Blätter-Animation verfeinern (abschaltbar)
- [ ] Themen anlegen, ändern, importieren und exportieren in der Oberfläche; Reihenfolge der Abschnitte einstellbar
- [ ] Hintergrundbilder für Einband und Papier im Thema (geprüft, nur aus dem Themenpaket)
- [ ] Tastaturbedienung und Druckansicht

### Phase 3 – Register ohne Google
- [ ] Kalender: Termine anlegen, verschieben, Wiederholungen, Erinnerungen
- [ ] Aufgaben: Fälligkeit, Priorität, erledigt
- [ ] Adressen: Register A–Z (je Sprache passendes Alphabet), Suche
- [ ] Notizen, Planer, Jahrestage

### Phase 4 – Google-Abgleich
- [ ] OAuth-Anmeldung, Token verschlüsselt speichern
- [ ] Kalender: Abgleich in beide Richtungen (Sync-Token, Änderungserkennung)
- [ ] Kontakte: People API, Geburtstage → Jahrestage
- [ ] Aufgaben: Google Tasks
- [ ] Konfliktregel festlegen (z. B. letzte Änderung gewinnt, mit Hinweis)
- [ ] Offline-Betrieb: Änderungen merken und später übertragen

### Phase 5 – Import der Altdaten
- [ ] CSV-Import je Bereich mit Feldzuordnung und Vorschau
- [ ] Zeichensatz prüfen (Umlaute, vermutlich Windows-1252)
- [ ] Wiederkehrende Termine und Jahrestage soweit möglich erkennen
- [ ] Option: importierte Daten direkt zu Google übertragen

### Phase 6 – Abschluss Version 1
- [ ] Datensicherung und Export (ICS, vCard, CSV)
- [ ] Tests im Alltag, Fehler beheben
- [ ] Anleitung für den eigenen Google-Zugang
- [ ] Erstes Release auf GitHub

## Offene Fragen

- Welche Kalenderansichten sind am wichtigsten (Woche als Doppelseite wie im Original?)
- Inhalt des CSV-Exports aus Organizer 6
- Weitere Sprachen außer Deutsch, Englisch und Arabisch
