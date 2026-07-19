# Resource Management & Dispatch Planning – System Design

**Status:** Draft / Shared Understanding  
**Goal:** Generic Open-Source System for planning and managing rentable resources.

## 1. Purpose & Core Idea
A generic system where resources are planned, requested, allocated, and returned.

*   **Users (Technicians)** request resources of a specific class for a job/context, use them, and return them.
*   **A central Dispatch Control (Dispatcher)** manages the resource pool, assigns concrete instances, and maintains an overview of all open requests.
*   The specific domain (e.g., calibration reference standards) is deliberately *not* part of the core. Domain-specific aspects (calibration validity, special validation rules) will be added later as optional plugins/extensions.
*   **Non-Goal:** History/Certificate evaluation (which resource was last used for what). This is a separate, standalone system.

## 2. Roles
| Role | Count (Sizing) | Workflow |
| :--- | :--- | :--- |
| **Technician** | ~150 | Mobile device + Laptop, offline-capable |
| **Dispatcher** | 3–4 concurrently | Online only, handles the actual dispatching |
| **Admin** | Few | Master data, user management |

*Both main roles are represented by multiple people → Status changes must be uniquely traceable to specific individuals (Audit Trail).*

### Cross-Technician Actions

A Technician may create requests on behalf of any `technician_id` and may request the return of any Allocation — not only their own. This is intentional: it supports collaborative on-site scenarios (e.g., handing off equipment between colleagues) and avoids ownership-enforcement at the API level.

**All such cross-technician actions are fully audit-safe**: every state-changing operation records the acting `actor_id` and `actor_role` in an immutable `AuditEvent` within the same database transaction. This provides a complete, tamper-proof trace of who acted on whose behalf.

## 3. Resource Lifecycle Rules

### Direct Transfer (Site-to-Site)

A resource does **not** have to return to the warehouse between two
deployments. If it is not reported defective/blocked, it may be
transferred directly from one deployment site to the next
(direct transfer).

* A full return cycle with inspection is **desired but not required**.
* Direct transfer is only permitted if the resource has **no active
  block** (not defective, not under mandatory inspection).
* At any point in time a resource has **at most one active allocation**
  (enforced by a unique partial index). On direct transfer, the previous
  allocation must be completed in the same transaction in which the
  next allocation becomes active — there is no gap and no overlap.
* A direct transfer produces audit events for both allocations
  (completion of the old, activation of the new).
* Direct transfers and all operative resource allocations are
  **exclusively** Dispatcher actions. Technician-to-technician transfer
  approvals are business-wise excluded.

### Return to Warehouse & Inspection

* When a resource physically returns to the warehouse
  (`shipped_back` → received), it **should** be inspected before
  becoming `available` again. Inspection is the default path.
* There is **no automatic transition** from `shipped_back` to
  `available`; making a resource available again is always an
  explicit dispatcher action (with audit trail).
* `shipped_back` counts as an **active** state for the
  single-active-allocation constraint: a resource in transit back
  cannot be allocated to a new request. Direct transfer is only
  possible *before* a return shipment is initiated.

## 4. Architecture (Hexagonal / Clean Architecture)

```text
┌─────────────────────────────────────────────┐
│  Adapters (interchangeable)                 │
│  - DB: PostgreSQL (Default), SQLite ready   │
│  - HTTP-API (REST + SSE)                    │
│  - Web-Dashboard (Dispatcher)                │
├─────────────────────────────────────────────┤
│  Ports (Interfaces)                         │
│  - ResourceRepository, RequestRepository    │
│  - EventPublisher, AuditWriter              │
├─────────────────────────────────────────────┤
│  Application Layer (Use Cases)              │
│  - CreateRequest, AllocateResource,         │
│    ReturnResource, SyncOutbox …             │
├─────────────────────────────────────────────┤
│  Domain Core (pure model, no external       │
│  dependencies) – Entities, State Machines   │
└─────────────────────────────────────────────┘
```

## 5. API Security

### Authentication & Authorization

*   **Authentication** answers "Who is the user?" — a valid Bearer token produces an authenticated `Principal` with `UserID` and `Role`.
*   **Authorization** answers "Is this user allowed to use this endpoint?" — checked per route based on the Principal's `Role`.
*   Middleware order is strictly: **Authentication → Role check (`requireRoles`) → Handler**. This order is enforced at route registration in `NewHandlerWithClock`.
*   A missing or invalid token always produces **401 Unauthorized** (never 403).
*   A valid Principal with a disallowed role produces **403 Forbidden** (never 401).
*   A missing Principal inside a handler signals a programming error (middleware incorrectly wired) and produces **500** to make the defect immediately visible.
*   Actor identity for AuditEvents comes **exclusively** from the authenticated Principal — never from the request body or headers.
*   Direct Transfer is reserved exclusively for the Dispatcher role.
*   Admin does **not** automatically have access to operative write actions.

### Role Canonical Values

All layers — domain model, HTTP authorization, token configuration, and database — use the same canonical role values:

| Domain constant | Value stored everywhere |
|:---|:---|
| `domain.ActorRoleTechnician` | `"technician"` |
| `domain.ActorRoleDispatcher` | `"dispatcher"` |
| `domain.ActorRoleAdmin` | `"admin"` |
| `domain.ActorRoleSystem` | `"system"` |

Note: "Dispatcher" is the single canonical term for the dispatch role.

### Permission Matrix

| Endpoint | Technician | Dispatcher | Admin |
|:---|:---:|:---:|:---:|
| `POST /api/v1/requests` | ✅ | ❌ | ❌ |
| `GET /api/v1/requests/{id}` | ✅ | ✅ | ✅ |
| `POST /api/v1/allocations/{id}/return-request` | ✅ | ❌ | ❌ |
| `POST /api/v1/resources/{id}/transfer` *(future)* | ❌ | ✅ | ❌ |

### Delegation & Stellvertreter-Aktionen (On-Behalf-Of)

*   **Feature by Design:** Techniker dürfen explizit Requests für andere Techniker anlegen oder Rückgaben für fremde Allocations initiieren (z. B. wenn ein Kollege vor Ort aushilft oder das Tablet ausfällt). Es gibt bewusst keine strikte Ownership-Prüfung, die dies blockiert.
*   **Audit Security & Non-Repudiation:** Um Missbrauch auszuschließen und Aktionen lückenlos nachzuverfolgen, trennt das System strikt zwischen dem **ausführenden Akteur (Actor)** und dem **fachlichen Ziel (Target)**.
*   Die Actor-Identität für Audit-Events wird **ausschließlich** aus dem authentifizierten Principal bezogen – niemals aus dem Request-Body oder den Headern[cite: 1].
*   Die `technician_id` in der Business-Payload (z. B. beim POST Request) definiert lediglich den Empfänger/Besitzer der Ressource.
*   **Ergebnis:** Wenn Techniker A eine Ressource für Techniker B anfordert, gehört der Request fachlich zu Techniker B. Das Audit-Log dokumentiert jedoch manipulationssicher Techniker A als Initiator der Transaktion.