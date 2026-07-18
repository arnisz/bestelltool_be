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

## 3. Architecture (Hexagonal / Clean Architecture)

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