# Front Proxy Groups Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add manually imported third-party front proxy nodes that admin can authorize and attach per customer assignment, then emit assignment-specific `dialer-proxy` groups in customer Mihomo subscriptions.

**Architecture:** Store front proxy nodes separately from VPSMonitor Clients, grant them to area managers or customers, and attach selected nodes to customer assignments. Subscription rendering reads the final assignment-level front proxy list and creates a select proxy group used by that assignment only.

**Tech Stack:** Go backend with SQLite store and net/http handlers, existing Mihomo parser/rendering helpers, React/TypeScript admin UI.

---

### Task 1: Backend Store And Model

- [ ] Add model structs for front proxy nodes, grants, and assignment selections.
- [ ] Add SQLite tables and indexes for `front_proxy_nodes`, `front_proxy_grants`, and `customer_assignment_front_proxies`.
- [ ] Add store methods to create/update/delete/list nodes, replace grants, validate assignment selections, and list selected nodes by assignment.
- [ ] Write store tests before implementation.

### Task 2: Subscription Rendering

- [ ] Replace the temporary fixed `IEPL -> 999` chain with assignment-level front proxy data.
- [ ] Create unique group names per customer assignment and inject third-party proxies into the generated YAML.
- [ ] Write subscription tests that prove only configured assignments get `dialer-proxy`.

### Task 3: Admin APIs

- [ ] Add `/api/v1/admin/front-proxies` CRUD endpoints.
- [ ] Extend customer and area-manager account requests with front proxy grants.
- [ ] Extend customer assignment requests with front proxy selections, scoped by admin/area-manager permissions.
- [ ] Add handler tests for area-manager scoping.

### Task 4: Frontend UI

- [ ] Add TypeScript types and API calls.
- [ ] Add an admin front proxy management panel/import form.
- [ ] Add grant selectors for area managers/customers and per-assignment front proxy selectors.
- [ ] Build and verify the web bundle.

