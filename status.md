# Project Status: Resource Planning System (Go Backend)

## 🎯 Current Focus
PostgreSQL-Adapter gegen die reale Raspberry-Pi-Testdatenbank validieren und Integrationstest-Abdeckung vervollständigen.

## ✅ Completed
- [x] System Architecture and Requirements defined (`systemdesign.md`).
- [x] AI Agent rules and architectural constraints defined (agents.md`).
- [x] Project Status tracking initialized (`status.md`).
- [x] Go-Modul initialisiert.
- [x] Hexagonale Verzeichnisstruktur erstellt.
- [x] Domain-Entitäten implementiert.
- [x] Domain-Zustandsautomaten implementiert.
- [x] Domain-Unit-Tests implementiert.
- [x] Application Ports definiert.
- [x] Unit-of-Work-Vertrag definiert.
- [x] Repository-Verträge für erste transaktionale Use Cases definiert.
- [x] AuditWriter- und IdempotencyStore-Ports definiert.
- [x] Erste transaktionale Use Cases implementiert.
- [x] Application-Unit-Tests mit In-Memory-Fakes implementiert.
- [x] PostgreSQL-Schema entworfen.
- [x] Initiale Up- und Down-Migrationen erstellt.
- [x] Audit- und Idempotency-Tabellen erstellt.
- [x] Fachliche Constraints und Indizes definiert.
- [x] Migrationen dokumentiert (`migrations/README.md`).
- [x] Migrationen technisch validiert, soweit in der Umgebung möglich (Go-Checks ausgeführt; PostgreSQL-/Docker-Lauf mangels verfügbarer Tools nicht ausführbar).
- [x] PostgreSQL-Connection-Pool-Adapter implementiert.
- [x] PostgreSQL-Unit-of-Work implementiert.
- [x] Transaktionsgebundene Repositories implementiert.
- [x] PostgreSQL-AuditWriter implementiert.
- [x] PostgreSQL-IdempotencyStore implementiert.
- [x] PostgreSQL-Integrationstests ergänzt (mit Skip bei fehlender `TEST_DATABASE_URL`).
- [x] Konfliktbehandlung in PostgreSQL-Repositories geschärft: reine Versionssprünge ohne fachliche Feldänderung werden als Optimistic-Locking-Konflikt behandelt (Request/Resource/Allocation).

## 🔄 In Progress
- [ ] PostgreSQL-Integrationstests gegen die Raspberry-Pi-Testdatenbank vollständig ausführen (lokal weiterhin Skip ohne `TEST_DATABASE_URL`).
- [ ] Reale PostgreSQL-Validierung mit gesetzter `TEST_DATABASE_URL` durchführen (aktuell alle Adapter-Integrationstests als SKIP, da Variable in dieser Umgebung nicht gesetzt).
- [ ] Erste HTTP-Use-Case-Anbindung und Server-Bootstrap vorbereiten.

## ⏭️ Next Steps (in order)
1. Server-Konfiguration definieren.
2. PostgreSQL-Pool im Composition Root verdrahten.
3. HTTP-Adapter mit Go 1.22 ServeMux erstellen.
4. Einheitliche Fehlerantworten definieren.
5. Erste Request- und Allocation-Endpunkte anbinden.
6. Authentifizierungs-Port vorbereiten.
7. SSE-Adapter vorbereiten.
8. End-to-End-Tests ergänzen.

## ⚠️ Known Issues / Tech Debt
- Derzeit keine zusätzlichen bekannten Laufzeitprobleme außerhalb der offenen In-Progress-Punkte dokumentiert.

## 📝 Rules for the AI Agent
- **READ THIS FILE FIRST** at the start of every session or task.
- **UPDATE THIS FILE** immediately when a task from "In Progress" or "Next Steps" is finished.
- Move completed items to the "Completed" list and keep the "Current Focus" sharply aligned with the current immediate goal.