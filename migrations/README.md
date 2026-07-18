### Überblick
- Die Migrationen unter `migrations/` sind die einzige Schemaquelle.
- Reihenfolge:
  - `000001_create_core_tables.*`: Kerntabellen für Aggregate und Beziehungen.
  - `000002_create_audit_and_idempotency.*`: Audit-Trail, Idempotency-Outcome-Replay, Append-only-Regel.
  - `000003_create_indexes.*`: Zugriffspfade, Dispatch-Indizes, partielle Exklusivitätsregel.

### Tabellen
- `users`: persistierbare Benutzerstammdaten (`technician`, `elz`, `admin`) mit Deaktivierungsflag.
- `resource_classes`: Klasse mit `metadata jsonb`.
- `resources`: konkrete Ressource inkl. Status, Sperrgrund, Halter, `valid_until`, `version`.
- `requests`: Anfrage inkl. Status, Execution-State, Wunschzeitraum, Kontext, `version`.
- `request_resource_classes`: normalisierte Zuordnung Request ↔ ResourceClass über `position` (Duplikate pro Klasse bleiben möglich).
- `allocations`: Zuordnung Request ↔ Resource inkl. Zeitfenster, Lifecycle-Zeitpunkten, `version`.
- `audit_events`: append-only Persistierung von Audit-Einträgen mit `server_recorded_at DEFAULT clock_timestamp()`.
- `idempotency_outcomes`: Outcome-Replay pro `client_action_id` inkl. gespeicherten Ergebnisfeldern (`status_code`, `payload`, `error_text`).

### ID-Strategie
- Aggregate- und Referenz-IDs sind `text` entsprechend den Domain-ID-Typen.
- Keine ID-Erzeugung über Sequenzen oder Defaults.
- `client_action_id` ist `text` (Port nutzt `string`, keine UUID-Vorgabe im Vertrag).

### Zeitstempelstrategie
- Fachliche Zeitpunkte werden als `timestamptz` gespeichert.
- Audit/Idempotency verwenden autoritative Datenbankzeit über `clock_timestamp()`.
- `client_occurred_at` und `client_seq` bleiben für Offline-Fälle nullable.

### Statusmodellierung und Constraints
- Statusfelder sind `text` mit expliziten `CHECK`-Constraints (keine PostgreSQL-Enums).
- Statuswerte werden exakt aus den Domain-Konstanten übernommen.
- `resources`: `status='blocked'` erzwingt `block_reason`; andere Status verbieten `block_reason`.
- `requests`: `blocked`/`partially_blocked` erzwingen nicht-leere `execution_note`; `executable` erzwingt leere Note.
- Zeitbereiche:
  - `requests`: `wish_until > wish_from`, falls beide gesetzt.
  - `allocations`: `planned_until > planned_from`.
- Optimistic Locking: `version bigint NOT NULL CHECK (version >= 1)` auf `requests`, `resources`, `allocations`.

### Löschregeln
- Zentrale Aggregate verwenden `ON DELETE RESTRICT`.
- Reine Beziehungstabelle `request_resource_classes` nutzt `ON DELETE CASCADE` auf `requests`.
- Keine Kaskadenlöschung für `audit_events`.

### Wichtige Indizes
- `resources`: `idx_resources_valid_until`, `idx_resources_dispatch_lookup`.
- `requests`: Status-, Techniker-, Kontext- sowie Status+Aktualisierungszeit-Index.
- `allocations`: Indizes für Request/Resource/Status/Planned-Until und Dispatch-Fenster.
- `uq_allocations_single_active_resource`: partieller Unique Index verhindert doppelte aktive Allocation pro Resource.
- `audit_events`: `(entity_type, entity_id, server_recorded_at)`, `(actor_id, server_recorded_at)`, `server_recorded_at`.
- `idempotency_outcomes`: `(actor_id, client_seq)` für potenzielle Replay-/Sync-Analysen.

### Audit Append-only
- Trigger `trg_audit_events_no_update` und `trg_audit_events_no_delete` blockieren Änderungen/Löschungen.
- Für kontrollierte spätere Migrationen kann die Regel temporär angepasst werden, indem Trigger/Funktion in einer eigenen Migration gezielt gedroppt/ersetzt und danach wiederhergestellt werden.

### Bewusst offene Entscheidungen
- Fristen-Policy bleibt offen: maßgeblich ist fachlich noch nicht entschieden (`client_occurred_at` vs. `server_recorded_at`).
- Techniker-Geräteidentität zur formalen Prüfung monotoner `client_seq` ist noch nicht abschließend modelliert.
- PostgreSQL-Adapter, transaktionsgebundene SQL-Abfragen und konkrete Repository-Implementierungen folgen im nächsten Arbeitsschritt.

### Modellierungsnotiz (konservative Kompatibilität)
- Domain speichert angeforderte ResourceClasses als Slice ohne explizite Mengenregel.
- `request_resource_classes` verwendet daher `position` statt `(request_id, resource_class_id)` als Schlüssel, um potenzielle Mehrfachvorkommen nicht stillschweigend zu verbieten.
- Rollenabbildung ist explizit harmonisiert: persistenter Audit-Wert ist `elz`.
- Kompatibilität: Adapter mappt Domain-`dispatcher` beim Schreiben explizit auf `elz`; Migration `000004_align_actor_role_elz.*` stellt bestehende Audit-Daten/Constraint darauf um.