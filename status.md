# Project Status: Resource Planning System (Go Backend)

## 🎯 Current Focus
PostgreSQL-Schema und Migrationen auf Basis der definierten Application-Ports und Use Cases entwerfen.

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

## 🔄 In Progress
- [ ] PostgreSQL-Schema und Migrationen entwerfen.

## ⏭️ Next Steps (in order)
1. PostgreSQL-Schema und Migrationen entwerfen.
2. PostgreSQL-Adapter für Repositories und Unit of Work implementieren.
3. Audit-Persistierung im selben Transaktionskontext anbinden.
4. IdempotencyStore im PostgreSQL-Adapter implementieren.
5. Erste HTTP-Use-Case-Anbindung im Adapter vorbereiten.
6. End-to-End-Tests für transaktionale Statusänderungen ergänzen.

## ⚠️ Known Issues / Tech Debt
- Root-`main.go` ist ein IDE-Template und nicht Teil der Hex-Arch-Laufzeit.

## 📝 Rules for the AI Agent
- **READ THIS FILE FIRST** at the start of every session or task.
- **UPDATE THIS FILE** immediately when a task from "In Progress" or "Next Steps" is finished.
- Move completed items to the "Completed" list and keep the "Current Focus" sharply aligned with the current immediate goal.