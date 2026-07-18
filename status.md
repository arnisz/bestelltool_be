# Project Status: Resource Planning System (Go Backend)

## 🎯 Current Focus
PostgreSQL-Adapter für Repositories und Unit of Work implementieren.

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

## 🔄 In Progress
- [ ] PostgreSQL-Adapter für Repositories und Unit of Work implementieren.

## ⏭️ Next Steps (in order)
1. pgx-basierte Connection-Pool-Anbindung erstellen.
2. PostgreSQL-Unit-of-Work implementieren.
3. Transaktionsgebundene Repositories implementieren.
4. AuditWriter im selben Transaktionskontext implementieren.
5. IdempotencyStore implementieren.
6. Integrationstests gegen PostgreSQL ergänzen.
7. Erste HTTP-Use-Case-Anbindung vorbereiten.

## ⚠️ Known Issues / Tech Debt
- Root-`main.go` ist ein IDE-Template und nicht Teil der Hex-Arch-Laufzeit.

## 📝 Rules for the AI Agent
- **READ THIS FILE FIRST** at the start of every session or task.
- **UPDATE THIS FILE** immediately when a task from "In Progress" or "Next Steps" is finished.
- Move completed items to the "Completed" list and keep the "Current Focus" sharply aligned with the current immediate goal.