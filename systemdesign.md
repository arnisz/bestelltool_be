# Resource Management & Dispatch Planning – System Design

**Status:** Draft / Shared Understanding  
**Goal:** Generic Open-Source System for planning and managing rentable resources.

## 1. Purpose & Core Idea
A generic system where resources are planned, requested, allocated, and returned.

*   **Users (Technicians)** request resources of a specific class for a job/context, use them, and return them.
*   **A central Dispatch Control (ELZ / Dispatcher)** manages the resource pool, assigns concrete instances, and maintains an overview of all open requests.
*   The specific domain (e.g., calibration reference standards) is deliberately *not* part of the core. Domain-specific aspects (calibration validity, special validation rules) will be added later as optional plugins/extensions.
*   **Non-Goal:** History/Certificate evaluation (which resource was last used for what). This is a separate, standalone system.

## 2. Roles
| Role | Count (Sizing) | Workflow |
| :--- | :--- | :--- |
| **Technician** | ~150 | Mobile device + Laptop, offline-capable |
| **Dispatch (ELZ)** | 3–4 concurrently | Online only, handles the actual dispatching |
| **Admin** | Few | Master data, user management |

*Both main roles are represented by multiple people → Status changes must be uniquely traceable to specific individuals (Audit Trail).*

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
  (completion of the old, activation of the new). Until decided
  otherwise, only the dispatcher (ELZ) may approve a direct transfer.

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
│  - Web-Dashboard (ELZ)                      │
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
