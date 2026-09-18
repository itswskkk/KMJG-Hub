## Resume Here

Last verified commit: `88f1cbe` (`feat: add project members view`)
Branch: `main`
Working tree after this checkpoint: **dirty, uncommitted** (implementation complete and verified;
committing/pushing was explicitly out of scope for this session — see Git status below).

Current implementation state:
- Connect Server: complete
- Local Auth / Sessions: complete
- Projects / Overview: complete
- Members view-only: complete, now with real presence dots
- **Presence / Authenticated WebSocket Foundation: complete (this checkpoint)**

No work is currently in progress.

Next checkpoint must be selected by the project owner.
Current candidates:
- Removing a Project Member Experience (member management)
- Tasks
- Project Chat (now unblocked — the WebSocket/event-envelope foundation exists)
- Work Status / Current Branch / Current Task

**Concurrency note for whoever picks this up:** during this session, another
Claude Code session (`kmjg-hub-af`) was independently working the same
checkpoint in the same working directory at the same time. Both sessions
detected it, compared status, and the user chose to have this session (the
one that had already completed and verified the server side) finish and own
the checkpoint; the other session stood down without further edits. No
known conflicting edits landed — flagging only so a reviewer isn't surprised
by unfamiliar rhythm in the diff, and so nobody assumes there is a second,
divergent copy of this work sitting somewhere.

# KMJG Hub — Implementation Progress

This file is a handoff/state snapshot for whichever agent (Codex, Claude Code,
or a future session) picks up work next. It describes implementation status
only.

**Precedence:** `docs/VISION.md`, `docs/PRD.md`, `docs/UX.md`, and
`docs/ARCHITECTURE.md` are always authoritative. This file must never be used
to introduce, change, or reinterpret product requirements — if anything here
conflicts with those docs, the docs win and this file should be corrected.

---

## Current Milestone / Vertical Slice

The first vertical slice (Connect Server → Login → Load Projects → Select
Project → Overview) is complete and was verified end-to-end against a real
PostgreSQL-backed server. The Members checkpoint built the Members screen on
top of it. **This session's checkpoint (Presence / Authenticated WebSocket
Foundation)** adds the real-time layer both of those needed and were
honestly missing, and wires real Online/Offline presence into Overview and
Members in place of the previous "not implemented yet" notes.

## Completed

### Checkpoint 4: Presence / Authenticated WebSocket Foundation (this checkpoint)

**Scope, per `docs/ARCHITECTURE.md` "Presence and Work Status Architecture"
and "Real-Time Communication":** an authenticated WebSocket layer, deriving
Online/Offline presence from live connections (never a stored boolean), and
distributing it only to Project members authorized to see it. Work Status,
Current Branch, and Current Task are explicitly separate concerns and are
**not** implemented this checkpoint — see "Known Issues / Blockers" below.

**Server (Go) — new packages:**

- **`internal/realtime`** (`envelope.go`, `client.go`, `hub.go`) — a small,
  transport-agnostic connection registry. `Envelope{V, Type, Data}` is the
  wire shape (`{"v":1,"type":"...","data":{...}}`, matching
  `docs/ARCHITECTURE.md`'s example and giving the protocol an explicit
  version field per "Real-Time Protocol Versioning"). `Client` wraps one
  authenticated connection (`UserID`, hashed `TokenHash`, a bounded 32-slot
  outbound queue, and a transport-supplied `closeConn` hook); `Enqueue` is
  non-blocking and disconnects a client outright if its queue is full
  (never blocks a broadcasting goroutine on a slow/broken consumer, never
  grows unbounded). `Hub` is a mutex-guarded `map[userID]set[*Client]`
  supporting multiple connections per user; `Register`/`Unregister` fire
  `OnUserOnline`/`OnUserOffline` callbacks exactly on the 0→1 / 1→0
  transition, never on every connect/disconnect. `Hub` also exposes
  `CloseByTokenHash` (immediate logout-driven disconnect), `Sweep` (periodic
  session revalidation), and `Shutdown` (clean Server shutdown) — none of
  which know anything about presence, keeping the registry reusable for a
  future feature (Chat, Tasks) that also needs "deliver this event to this
  user's live connections."
- **`internal/presence`** (`presence.go`) — the presence domain. Defines
  `presence.snapshot` (full current state for one Project, sent to a single
  newly-registered connection) and `presence.updated`
  (`{project_id, user_id, online}`, satisfying this checkpoint's
  requirement that every presence event be unambiguous about what changed,
  where, and for whom). `Service.HandleUserOnline`/`HandleUserOffline` are
  wired as a `Hub`'s transition callbacks; both derive the broadcast
  audience *only* from a narrow `ProjectMembership` interface
  (`ProjectIDsForUser`, `MemberUserIDs`) the presence package defines for
  itself — never from anything a Client claims. The user whose presence
  just changed is excluded from their own broadcast (they already know
  their own state; including them raced against their own "connected"
  ack/snapshot on the very connection that triggered the event — caught by
  a test failure during development, see Implementation Decisions).
  `Service.SendSnapshot` pushes one `presence.snapshot` per Project the
  connecting user belongs to, addressed to that one new connection, so a
  fresh or reconnecting Client always gets authoritative current state
  (`docs/ARCHITECTURE.md` "Reconnection") instead of assuming nothing
  changed while it was away.

**Server (Go) — changes to existing packages:**

- **`internal/project`** — `Repository` gained `ListMemberUserIDs(ctx,
  projectID) ([]string, error)`; `Service` gained `ProjectIDsForUser` (thin
  wrapper over the existing `ListForUser`) and `MemberUserIDs` (thin wrapper
  over the new repo method). These two methods are the entire server-side
  capability the presence system needed and didn't have — no new schema,
  no new query beyond a one-column `SELECT user_id FROM project_members
  WHERE project_id = $1`. Implemented in
  `internal/store/postgres/projects.go`; the in-memory fakes in
  `internal/project/service_test.go` and
  `internal/httpapi/projects_test.go` (`fakeRepo`, `fakeProjectRepo`) were
  updated to implement the new interface method, and `fakeProjectRepo`
  gained a **test-only** `addMember` helper (there is still no product-facing
  way to add a second Project member — see Known Issues).
- **`internal/auth`** — `Service` gained `SessionActive(ctx, tokenHash)
  bool`, wrapping the existing `Sessions.GetActiveByTokenHash`. This is the
  one new capability the real-time layer needed from auth: a way to
  periodically revalidate an already-open WebSocket's session by its
  *hash* (never the raw token) against authoritative session state, per
  `docs/ARCHITECTURE.md` "Logout and Revocation" — "a revoked or expired
  session must no longer authorize ... WebSocket connections."
- **`internal/httpapi`** — new `websocket.go` (the transport layer: gorilla
  websocket upgrade, heartbeat/read-write pumps, auth-as-first-message) and
  `websocket_test.go` (real `httptest.NewServer` + real gorilla client
  integration tests). `Handlers` gained `Realtime *realtime.Hub`,
  `Presence *presence.Service`, and an `AuthTimeout time.Duration` field
  (see race-condition note below). `server.go` mounts `GET /api/v1/ws`
  **outside** `requireAuth` (browsers cannot set an `Authorization` header
  on a WebSocket upgrade request, so this connection authenticates itself
  via its first application message instead — see protocol section below).
  `auth.go`'s `handleLogout` now also calls
  `h.Realtime.CloseByTokenHash(auth.HashSessionToken(authed.Token))` so an
  explicit logout drops that session's WebSocket connections immediately
  rather than waiting for the periodic sweep.
- **`internal/app`** — wires a `realtime.Hub` and `presence.Service`
  together (`hub.OnUserOnline = presenceService.HandleUserOnline`, etc.),
  starts a background goroutine that calls `hub.Sweep` every 60s using
  `authService.SessionActive` (the bounded-delay fallback for session
  expiry, which — unlike logout — the Server has no single event to react
  to), and `App.Close()` now calls `hub.Shutdown()` before closing the
  database pool so no WebSocket connection or goroutine outlives Server
  shutdown.

**WebSocket authentication protocol:** the Client opens `GET /api/v1/ws`
unauthenticated, then its **first message** must be
`{"v":1,"type":"auth","data":{"token":"<session token>"}}`, validated via
the same `auth.Service.CurrentUser` used by every HTTP request. This
(rather than a `?token=...` query parameter) is deliberate: a browser's
native WebSocket API cannot set an `Authorization` header on the upgrade
request, and a token in the URL would otherwise land in Server access logs
and any intermediate proxy's logs, which `docs/ARCHITECTURE.md` "Session
Security" rules out ("never be written to application logs"). A connection
that never sends a valid auth message within `AuthTimeout` (10s in
production, shortened only in tests) is closed without ever being
registered. An invalid/expired/revoked token gets a
`{"type":"error","data":{"code":"unauthorized",...}}` frame, then the
connection is closed — no partial registration, no distinguishable error
that would let a client tell "wrong token" from "expired token" from
"never existed."

**Heartbeat / liveness:** standard gorilla/websocket pattern —
Server pings every 54s (`pingPeriod`), Client's (browser's, automatically —
no application code needed) pong resets a 60s read deadline
(`pongWait`) via `SetPongHandler`; a connection that stops responding is
detected and torn down within `pongWait` of its last pong. `SetReadLimit`
bounds every frame (including the auth message) to 4096 bytes.

**Client (React/TypeScript):**

- **`src/lib/realtimeClient.ts`** — new. Framework-agnostic `RealtimeClient`
  class: opens the WebSocket, sends the auth message on open, parses
  `connected`/`presence.snapshot`/`presence.updated`/`error` envelopes,
  and reconnects with exponential backoff + jitter (1s → 30s cap) on any
  unexpected close. `toWebSocketUrl` converts the Client's existing
  selected Server address into a `ws`/`wss` endpoint
  (`https→wss`, `http→ws`), never a hardcoded host. Every internal
  callback (`onopen`/`onmessage`/`onclose`) checks `this.closed` and that
  the event belongs to the currently-tracked socket (`this.ws !== ws`)
  before touching state — the specific guard needed so React Strict
  Mode's mount→cleanup→mount double-invoke (dev only) can never let a
  superseded socket deliver a stale event. Receiving an `error` envelope
  (session rejected) stops the client permanently instead of retrying with
  a token the Server has already rejected.
- **`src/features/presence/PresenceProvider.tsx`** — new. React Context
  provider owning the single authenticated real-time connection for the
  whole authenticated app session. Exposes `useProjectPresence(projectId)`
  returning `{ status, isOnline(userId) }`, where `isOnline` returns
  `undefined` (never a guessed `false`) whenever the connection isn't
  currently `"open"` — the mechanism behind "don't present stale/fake
  presence while disconnected."
- **`src/App.tsx`** — restructured so `<PresenceProvider>` wraps the whole
  screen switch at a single, stable position, with `serverUrl`/`token`
  derived from the current screen (`null` before authentication). This is
  the specific choice that keeps the connection alive across Server
  Home → Create Project → Project Workspace navigation (screens that
  React would otherwise fully unmount/remount, tearing the socket down and
  reopening it on every click) while still reconnecting for real state
  changes: login, logout, or a Server change. `onSessionExpired` routes a
  Server-rejected session back to that Server's Login screen, the same way
  an HTTP 401 already does elsewhere in the app.
- **`src/features/projects/Overview.tsx`** — the "Online presence is not
  implemented yet" note is gone. When the real-time connection is `"open"`,
  Overview shows the real `Members Online: N / M` count
  (`docs/UX.md` "Project Overview": "Members Online \n 3 / 4"); while
  connecting/reconnecting, it shows the member count plus a "Connecting to
  real-time presence…" note instead of a stale or fabricated count.
- **`src/features/projects/Members.tsx`** + **`Members.css`** — each member
  row now shows a real presence dot (green `●` online / hollow `○`
  offline) once the connection is `"open"`; before that, no dot is drawn
  and a neutral note explains why. The Member Detail overlay's blanket
  "Presence, work status, current branch, and current task are not
  implemented yet" note was narrowed to "Work status, current branch, and
  current task are not implemented yet" (Presence itself is real now) and
  gained an explicit `"● Online"` / `"○ Offline"` /
  `"Presence unknown (reconnecting…)"` line. The four quick actions (Chat,
  Send File, View Branch, View Current Task) remain disabled — untouched,
  still out of scope.

## Completed (previous checkpoints)

### Checkpoint 3: Members

**Scope decision made before starting, per the project owner's explicit
choice when asked:** `docs/UX.md` has two related but distinct sections —
"Members Experience" (pure viewing: who's in the project, their role) and
"Removing a Project Member Experience" (Owner/Admin can remove a member,
with confirmation). The project owner's checkpoint instructions were
ambiguous about whether "authorization clearly separated by Owner/Admin/
Member" meant just structuring the domain for future role checks, or meant
actually implementing the one place PRD roles concretely diverge (remove
member). Asked directly; the answer was **view-only** — build "Members
Experience" only, defer "Removing a Project Member Experience" (and its
UX-mandated "also remove repository access" checkbox, which needs Git
integration to mean anything) to a future member-management checkpoint.

**No backend or database changes this checkpoint.** `GET
/api/v1/projects/{id}` (built in the previous checkpoint) already returns
each member's `id`, `username`, and `role` inside `ProjectDetail.members` —
exactly what `docs/UX.md` "Members Experience" needs for the fields we
actually have. Viewing the member list is already open to every Project
member regardless of role per `docs/PRD.md` ("Member can: View Project
members and their presence status"), and that was already true by
construction (`GetDetailForUser`'s membership check, not a role check).
Adding a second endpoint that returns the same data shaped slightly
differently would have been pure duplication, so the Members screen reuses
the existing `getProject` call already made by `ProjectWorkspace`.

**Client (React/TypeScript) — all of this checkpoint's work:**

- **`src/features/projects/Members.tsx`** + **`Members.css`** — new. Full
  member list (per `docs/UX.md` "Members Experience"): username + role per
  row, with an honest "Online presence and work status are not implemented
  yet" note instead of the presence dots (`●`/`○`) the UX mockup shows
  (superseded by real presence dots in the Presence checkpoint above) —
  rendering a fake online/offline indicator would be exactly the "fake
  data" the checkpoint instructions ruled out, since there is no presence
  system at all yet. Clicking a member opens a compact **Member Detail**
  overlay (`docs/UX.md` "Member Details"): username, role, an honest note
  that Presence/Work Status/Current Branch/Current Task aren't implemented,
  and all four documented quick actions (Chat, Send File, View Branch, View
  Current Task) rendered as real, visible, `disabled` buttons with a
  tooltip naming the specific unbuilt feature each depends on (Direct
  Messages, Direct File Transfer, Git integration, Tasks respectively) —
  same disabled-not-omitted treatment used for the GitHub login option and
  the Project Workspace sidebar stubs in previous checkpoints. Verified live
  that a disabled quick-action button is inert (click does nothing, no
  console error).
- **`src/features/projects/ProjectWorkspace.tsx`** — restructured from
  "Overview is the only real section, everything else in the sidebar is a
  disabled `<span>`" into a small `Section` (`"overview" | "members"`)
  state: Overview and Members are now both real, clickable sidebar buttons
  that switch content instantly using the project detail data already
  fetched once (no second network round-trip when switching sections);
  Chat/Tasks/Git/Files/Developer Tools remain disabled `<span>`s exactly as
  before — untouched, since none of them are in scope for this checkpoint.
- **`src/features/projects/Overview.tsx`** — simplified the Members section
  from a full inline roster (a previous-checkpoint addition that went
  further than the UX mockup, made before a dedicated Members screen
  existed) down to a member count + "View Members" button that switches
  `ProjectWorkspace` to the Members section. This matches
  `docs/UX.md`'s actual Overview mockup ("Members Online 3/4" — a summary
  stat, not a roster) more closely, and removes the duplication of showing
  the same per-member role list in two places now that Members is real.
  Removed the now-unused `.overview__members`/`.overview__member-role`
  CSS rules from `Overview.css`.

### Checkpoint 2: Load Projects → Select Project → Overview

**Fix carried in from the previous checkpoint's verification** (flagged by
the project owner, authorized as a small in-scope fix):
`auth.Service.CurrentUser` used to return the same `ErrInvalidCredentials`
for both "wrong login password" and "session token missing/expired/revoked".
Those are different situations. Added a distinct `auth.ErrSessionInvalid`
sentinel (`internal/auth/service.go`); the HTTP layer now maps it to a
separate `401 {"code":"invalid_session", ...}` instead of reusing
`invalid_credentials`. `POST /api/v1/auth/logout` and `GET
/api/v1/auth/session` tests updated to assert the new code; verified live
against the real server (see Tests section) — `curl`ing `/auth/session`
with a just-revoked token now returns `invalid_session`, not
`invalid_credentials`.

**Server (Go) additions:**

- **`internal/project`** — new domain package (`project.go`, `validate.go`,
  `service.go`). `Role` (`owner`/`admin`/`member`, per `docs/PRD.md` "Roles
  and Permissions"), `Project`, `Summary` (list-view + member count +
  viewer's role), `Member`, `Detail` (project + full member list).
  `Service.Create` / `List` / `GetDetail`. `Repository` interface kept
  independent of `auth`'s (no cross-package coupling for a 4-line
  `ValidationError` type — see Implementation Decisions).
- **`internal/store/postgres/migrations/0002_projects.sql`** — `projects`
  and `project_members` tables. `project_members.role` is a `TEXT CHECK
  (role IN (...))` column, not a Postgres `ENUM` type (enums are painful to
  alter later; a check constraint is simpler for v1 and easy to migrate off
  if needed).
- **`internal/store/postgres/projects.go`** — `ProjectRepository`.
  `CreateWithOwner` runs the project insert + owner membership insert in one
  transaction, so "creator becomes Owner" (`docs/PRD.md` "Project
  Creation") is atomic — never a project that exists with zero members.
  `GetDetailForUser`'s membership check is baked into the `JOIN` itself
  (`JOIN project_members pm ON ... AND pm.user_id = $2`), so a non-member
  and a nonexistent project both fall out as "no rows" → `project.ErrNotFound`
  → HTTP 404, without a separate authorization check that could be
  forgotten. A malformed (non-UUID) project ID is also mapped to
  `ErrNotFound` (Postgres SQLSTATE `22P02`) rather than surfacing as a 500.
- **`internal/httpapi/context.go`** — new `requireAuth` middleware +
  `authContext` (resolved user + raw bearer token) carried via
  `context.Context`. Refactored `handleLogout` and `handleCurrentSession` to
  use it instead of each doing their own `bearerToken` + `CurrentUser` call
  (was duplicated in 2 places, would have become 5 with Projects — now
  shared in one place, per `docs/ARCHITECTURE.md` "every protected Server
  operation must verify the authenticated user's current permissions").
- **`internal/httpapi/projects.go`** — handlers + DTOs. New authenticated
  routes:
  - `POST /api/v1/projects` — `{name, description?}` → 201 with the created
    project (`member_count: 1`, `role: "owner"`).
  - `GET  /api/v1/projects` — `{"projects": [...]}`, the caller's projects
    with member count and their role in each.
  - `GET  /api/v1/projects/{id}` — full detail: project info + member list
    (username + role). 404 if the project doesn't exist or the caller isn't
    a member.
- **`internal/app/app.go`** — wires `project.Service` (backed by
  `ProjectRepository`) into `Handlers`.

**What was deliberately *not* built server-side, and why** (per the "ทำ
backend เท่าที่จำเป็น" / "อย่าขยายscope" instructions):
- **Join Project (invite links/codes).** `docs/ARCHITECTURE.md` "Project
  Invitation Architecture" is a substantial separate piece (expiring
  Direct Invitations, invite codes with use limits, real-time invite
  events). Nothing in the Load-Projects→Overview slice requires it — a
  freshly-registered user can create their own project via `POST
  /api/v1/projects` to have something to select. Server Home's "Join
  Project" button is present (per `docs/UX.md`) but disabled with a "Not
  Available Yet" label, same treatment as the GitHub login stub. **Still
  true after the Presence checkpoint** — see Known Issues below.
- **Presence / online status (WebSocket).** Implemented in the Presence
  checkpoint above.
- **Tasks, Git activity, Project activity feed.** Each is its own
  `docs/ARCHITECTURE.md` section (Task Architecture; Git Provider
  Integration; Messaging and Real-Time Data Architecture) with real
  persistent data models. Overview's corresponding UX sections render with
  an honest "not implemented yet" note rather than invented data or an
  invented "feature availability" API flag.
- **Git repository connection.** Same reasoning — GitHub integration is
  explicitly deferred (see the previous checkpoint's Login-screen stub).
  `CreateInput` has no repository fields; every project created this
  checkpoint is the "Set Up Later" path from `docs/UX.md` "Project Creation
  Experience", which is a fully valid v1 Project state per `docs/PRD.md`.

**Client (React/TypeScript) additions:**

- **`src/lib/apiClient.ts`** — added `ProjectSummary`/`ProjectMember`/
  `ProjectDetail` types, `listProjects`/`createProject`/`getProject`
  (all send `Authorization: Bearer <token>`), and `isSessionExpired(err)` —
  a helper every new screen uses to detect a 401 (missing/expired/revoked
  session, now distinguishable via the `invalid_session` code from the fix
  above) and route the user back to Login instead of showing a raw error.
- **`src/features/server-home/ServerHome.tsx`** — per `docs/UX.md` "Server
  Home": "Your Projects" list (name + member count + Open), empty state
  ("You don't have any Projects yet."), "+ Create Project", "Join Project"
  (disabled stub), and a working **Log Out** button (calls the real
  `POST /api/v1/auth/logout`, then returns to Login regardless of whether
  the request succeeds — a token the Client is discarding is not worth
  blocking the UI over).
- **`src/features/projects/CreateProject.tsx`** — per `docs/UX.md`
  "Project Creation Experience": Name, Description (optional), and the
  Repository radio group (Create New GitHub Repo / Connect Existing — both
  disabled stubs; "Set Up Later" pre-selected and the only usable option).
- **`src/features/projects/ProjectWorkspace.tsx`** + **`Overview.tsx`** —
  per `docs/UX.md` "Project Workspace" / "Project Overview": sidebar nav
  (Overview active; Chat/Tasks/Git/Files/Members/Developer Tools rendered
  but disabled — same screen, same UX section, so same stub treatment as
  GitHub); "← Server Home" control; Overview shows the real member list
  with roles, and honest "not implemented yet" notes for Repository/Tasks/
  Git Activity/Project Activity/online presence.
- **`src/App.tsx`** — extended the `Screen` state machine:
  `server-home → create-project → project` (was a dead-end "authenticated"
  placeholder before). Renamed `authenticated` → `server-home`
  accordingly.
- **Bug fix, unrelated to Projects:** `src/App.css`'s dark-mode rule
  targeted `input, button` but not `textarea` — the app had no `<textarea>`
  until `CreateProject`'s Description field, which rendered with a
  jarring white background in dark mode. Added `textarea` to both the base
  and `prefers-color-scheme: dark` rules. Caught via the live browser
  walkthrough below, fixed immediately.
- **Deliberately not built:** a persistent nav bar for Friends / Direct
  Messages / Notifications / Profile on Server Home. Unlike the GitHub
  button or the Project Workspace sidebar items (which are stub-but-present
  because they belong to the *same UX section* as what this slice builds),
  Friends/DMs/Notifications/Profile are entire separate PRD features with
  no relationship to Projects. Adding disabled stub chrome for them felt
  like scope creep rather than the "same screen, deferred sub-option"
  pattern the other stubs follow — can be added when those features'
  checkpoints actually start.

## In Progress

- Nothing in-flight.
- **This session's checkpoint (Presence / Authenticated WebSocket
  Foundation) is implemented and fully verified but intentionally left
  uncommitted**, per explicit instruction for this session. See "Git
  status" in the handoff summary / commit it yourself when ready.

## Files Changed

**This checkpoint (Presence / Authenticated WebSocket Foundation):**

Server, new:
- `server/internal/realtime/envelope.go`, `client.go`, `hub.go`,
  `hub_test.go` — the connection registry.
- `server/internal/presence/presence.go`, `presence_test.go` — the
  presence domain (snapshot + updated events, Project-scoped broadcast).
- `server/internal/httpapi/websocket.go`, `websocket_test.go` — the
  authenticated WebSocket endpoint, heartbeat/liveness, and its
  integration tests.

Server, modified:
- `server/internal/project/project.go`, `service.go`, `service_test.go` —
  `ListMemberUserIDs` / `ProjectIDsForUser` / `MemberUserIDs`.
- `server/internal/store/postgres/projects.go` — `ListMemberUserIDs` impl.
- `server/internal/auth/service.go` — `SessionActive`.
- `server/internal/httpapi/server.go` — `Handlers.Realtime`/`.Presence`/
  `.AuthTimeout`, `GET /api/v1/ws` route.
- `server/internal/httpapi/auth.go` — logout closes matching WebSocket
  connections.
- `server/internal/httpapi/auth_test.go`, `projects_test.go` — test
  fixtures updated for the new `Repository` method and to wire up a real
  `Hub`/`Presence.Service`; `fakeProjectRepo` gained a test-only
  `addMember` helper.
- `server/internal/app/app.go` — wires the Hub/presence service, starts
  the session-sweep goroutine, closes the Hub on shutdown.
- `server/go.mod`, `go.sum` — added `github.com/gorilla/websocket`.

Client, new:
- `client/src/lib/realtimeClient.ts` — the WebSocket client.
- `client/src/features/presence/PresenceProvider.tsx` — React context/hook.

Client, modified:
- `client/src/App.tsx` — mounts `PresenceProvider` at a stable position.
- `client/src/features/projects/Overview.tsx` — real online count.
- `client/src/features/projects/Members.tsx`, `Members.css` — real
  presence dots + narrowed not-implemented note.

No database migration. No changes to `docker-compose.yml`, `.env.example`,
or any file outside `server/` and `client/src/`.

**Previous checkpoint (Members) — unchanged this session:**
- `client/src/features/projects/Members.tsx`, `Members.css` — original
  (view-only) version; see this checkpoint's diff on top of it above.
- `client/src/features/projects/ProjectWorkspace.tsx` — sidebar
  `overview`/`members` section switch.
- `client/src/features/projects/Overview.tsx`, `Overview.css` — original
  count + "View Members" version; see this checkpoint's diff on top.

**Previous checkpoint (Load Projects → Select Project → Overview) —
unchanged this session:**
- `server/internal/auth/service.go` — `ErrSessionInvalid` (unrelated to
  this checkpoint's `SessionActive` addition, same file).
- `server/internal/auth/service_test.go`, `server/internal/httpapi/auth.go`,
  `context.go`, `server.go`, `projects.go`, `projects_test.go`,
  `auth_test.go` — see this checkpoint's diffs on top of these same files.
- `server/internal/project/` — `project.go`, `validate.go`, `service.go`,
  `service_test.go`.
- `server/internal/store/postgres/projects.go`,
  `migrations/0002_projects.sql`.
- `server/internal/app/app.go`.
- `client/src/lib/apiClient.ts`, `client/src/features/server-home/`,
  `client/src/features/projects/{CreateProject.tsx,CreateProject.css,
  ProjectWorkspace.tsx,ProjectWorkspace.css,Overview.tsx,Overview.css}`,
  `client/src/App.tsx`, `client/src/App.css`.

**Original checkpoint** (Connect Server → Login; unchanged this session —
see git history / the prior version of this file for full detail): all of
`server/` except the additions above, `docker-compose.yml`, `.env.example`,
root `.gitignore`, `client/src/features/auth/`, original `apiClient.ts`.

## Tests / Build Checks (most recent)

- **This checkpoint (Presence / Authenticated WebSocket Foundation):**
  - `go build ./...`, `go vet ./...`, `gofmt -l .` — all clean, whole
    server module.
  - `go test ./...` — all packages pass. New coverage:
    - `internal/realtime` (`hub_test.go`, no network server needed): one
      connection makes a user Online; last connection disconnecting makes
      them Offline; one of several connections disconnecting does *not*;
      `Unregister` is idempotent; `SendToUser` delivers to every
      connection for a user and nobody else; a slow consumer (outbound
      queue full) gets disconnected instead of blocking the sender;
      `CloseByTokenHash` closes only the matching connection;
      `Sweep` closes only sessions a supplied validator rejects;
      `Shutdown` closes everything; envelope round-trip; a concurrent
      register/unregister/send stress test (also run under `-race`).
    - `internal/presence` (`presence_test.go`, no network server needed):
      the core cross-Project isolation test (`user-a` in Project X with
      `user-b` and Project Y with `user-c`; connecting `user-a` notifies
      `user-b` about X only and `user-c` about Y only, and an unrelated
      `user-d` sharing no Project learns nothing at all — same for the
      Offline transition on disconnect); `SendSnapshot` reflects exactly
      who is currently online; multiple connections from the same user
      produce exactly one Online and one (eventual) Offline broadcast, not
      one per connection.
    - `internal/httpapi` (`websocket_test.go`, real `httptest.NewServer` +
      real `gorilla/websocket` client): unauthenticated connection
      (no/garbage auth message) rejected; invalid token rejected;
      **revoked session rejected** (register → logout → attempt to
      authenticate the WebSocket with the now-revoked token → rejected);
      valid session accepted with a `connected` ack carrying the protocol
      version; `presence.snapshot` correctly scoped to the connecting
      user's own Project; **an authorized member receives another
      member's presence.updated, an outsider registered on the same
      Server but not a member of that Project receives nothing**;
      **logout closes an already-open WebSocket connection** (verified via
      a real HTTP logout call while the socket is open); oversized message
      (over the 4096-byte read limit) closes the connection; a malformed
      post-auth message is silently ignored without crashing the
      connection; an unauthenticated connection that sends nothing at all
      is closed once its auth timeout elapses (timeout shortened via a
      per-`Handlers` field for the test, not a shared mutable var — see
      Implementation Decisions/race note below).
  - `go test -race ./...` — clean, whole server module, run four times
    (including immediately after finding and fixing one real, reproducible
    race — see below) with no further findings.
  - **One race condition found and fixed during this checkpoint:** the
    WebSocket auth-timeout test initially shortened a shared
    `var httpapi.AuthTimeout` for its duration and restored it afterward.
    `go test -race` caught a genuine data race — the test's `t.Cleanup`
    writing the var back to its original value while a still-finishing
    server goroutine from an *earlier* test's `httptest.Server` (hijacked
    WebSocket connections are not tracked by `http.Server.Shutdown`, so
    `server.Close()` does not wait for them) concurrently read it. Fixed by
    moving `AuthTimeout` from a package-level `var` to a field on
    `Handlers` (set once per test's own router instance, never shared,
    never mutated after `NewRouter` starts serving) — a real bug the race
    detector was specifically supposed to catch, not a flaky/ignorable
    finding.
  - `npm run build` (`tsc && vite build`) in `client/` — passes, no type
    errors, across every edit in this checkpoint.
  - `git diff --check` — clean, no whitespace errors.
  - **Real Docker/PostgreSQL environment note:** this session's sandbox had
    neither Docker nor a Go toolchain preinstalled (differs from the
    previous checkpoint's environment/assumption). Go 1.27.1 was installed
    to the user's home directory (no `sudo`, no system-wide change) so
    `go build`/`vet`/`test` could run for real rather than being skipped.
    PostgreSQL 16 was installed the same way — `apt-get download` (fetches
    `.deb` packages without installing/root) + `dpkg-deb -x` (extracts
    without running Debian's install scripts) into a user-owned prefix,
    then `initdb`/`pg_ctl` run directly as the current user on a
    non-privileged port (5544) with its own data directory under the
    user's home. This is a real, unmodified PostgreSQL 16 server — not a
    stub or an in-memory substitute — just installed without root and
    without Docker. No project files, `docker-compose.yml`, or `.env.example`
    were changed to make this work; it is purely local sandbox setup,
    documented here so a future session understands why Docker wasn't used
    and doesn't need to repeat the investigation.
  - **Live end-to-end verification against the real Go server (built from
    this checkpoint's code) and the real local PostgreSQL above, plus the
    real Vite dev server and a real Chrome browser (two tabs, two real
    registered accounts):**
    - `docker`/`go`/`node` toolchain check → confirmed the above, installed
      Go and PostgreSQL as described.
    - Started the real server (`go run ./cmd/server`) against the real
      Postgres instance; migrations `0001` and `0002` applied cleanly
      (no new migration this checkpoint).
    - `curl`-registered a user and connected a raw WebSocket client
      (a small Node script using the platform `WebSocket` global) directly
      against `ws://localhost:8099/api/v1/ws`: sent the `auth` message,
      received a real `connected` ack and a real `presence.snapshot`
      showing itself online in its own just-created Project — confirming
      the protocol end-to-end below the browser layer too.
    - Confirmed logout closes an open WebSocket in practice, not just in
      the test suite: opened a socket, authenticated, then called
      `POST /api/v1/auth/logout` over real HTTP with the same token from a
      separate script — the socket closed within 16ms.
    - **Two-browser-tab walkthrough**, using the real Client UI (not curl)
      end to end: registered two real accounts through the running Vite
      dev server (`korn_e2e`, `meran_e2e`) via the real Register/Login
      screens's underlying API, created a real Project as `korn_e2e`
      through the real Create Project screen, and — since there is still
      no Join/Invite flow to reach a second real membership through the
      UI — added `meran_e2e` as a second member of that one Project with a
      **single direct SQL `INSERT` into `project_members`** on the local
      Postgres instance. This is temporary verification data only: it
      changed no product code, no schema, and was performed exactly once,
      documented here as instructed. Then, entirely through the real
      Client UI in two browser tabs:
      - Logged in as `korn_e2e`, opened the shared Project: Overview showed
        **"Members Online: 1 / 2"** and Members showed `korn_e2e` with a
        green online dot and `meran_e2e` with a hollow offline dot — both
        correct and both real (no page reload between login and this
        state).
      - Logged in as `meran_e2e` in the second tab and opened the same
        Project: **without touching or reloading the first tab**, its
        Members list updated `meran_e2e`'s dot from hollow to filled live,
        and its Overview count updated to **"Members Online: 2 / 2"** —
        genuine push-based real-time delivery, not polling.
      - Closed the second tab (simulating a normal disconnect): the first
        tab's Overview count dropped back to **"1 / 2"** within about two
        seconds, with no interaction on that tab.
      - Killed the real server process entirely (simulating an outage)
        while the first tab stayed open and logged in: Overview correctly
        stopped showing any online count and displayed
        **"Connecting to real-time presence…"** instead — never a stale or
        fabricated `"1/2"`/`"0/2"`, matching the checkpoint's explicit
        "unknown vs. confirmed" requirement.
      - Restarted the real server on the same port: **without any user
        action or page reload**, the Client's backoff-based reconnect
        logic re-established the WebSocket, re-authenticated, and Overview
        recovered to the correct **"1 / 2"** on its own within a few
        seconds.
      - Opened the Member Detail overlay for `korn_e2e` (self): showed
        **"● Online"**, the narrowed "Work status, current branch, and
        current task are not implemented yet" note, and all four quick
        action buttons (Chat, Send File, View Branch, View Current Task)
        still visibly disabled — presence is real, nothing else was faked.
      - Clicked **Log Out**: returned cleanly to the Login screen; no
        console errors at any point across the entire walkthrough in
        either tab (checked explicitly via the browser's console after
        login, after the live cross-tab update, after the outage/reconnect
        sequence, and after logout).
    - Test accounts/data created during this verification
      (`korn_e2e`/`meran_e2e`/`livecheck`/`logoutcheck` and their
      "E2E Presence Check"/"Live Check Project" projects) live only in the
      local sandbox Postgres instance described above, not in any shared or
      persistent deployment database.

- **Previous checkpoints' verification, for reference — unchanged this
  session:** see the prior version of this file / git history. Summary:
  Members checkpoint verified live against a real running `docker compose`
  deployment with a single-member Project (multi-member Members rendering
  was reasoned about from the code but not exercised live at the time —
  it *has* now been exercised live, for real, in this checkpoint's
  two-tab walkthrough above, since Presence needed a real second member to
  verify against). Load Projects → Overview checkpoint verified live
  against real Docker/PostgreSQL with full register/create/list/get/404/
  validation/logout coverage.

## Implementation Decisions Made

**This checkpoint (Presence / Authenticated WebSocket Foundation):**

- **Auth-as-first-message, not a `?token=` query parameter.** Browsers'
  native `WebSocket` API cannot set an `Authorization` header on the
  upgrade request, so *some* non-header mechanism was unavoidable. A query
  parameter was rejected specifically because
  `docs/ARCHITECTURE.md` "Session Security" prohibits tokens in logs, and
  URLs (including query strings) routinely end up in Server access logs
  and intermediate proxy logs even when application code never explicitly
  logs them. Sending the token as the first application-level WebSocket
  message avoids that class of leak entirely.
- **No client-originated subscription message.** The checkpoint
  instructions explicitly preferred this when authorized Projects can be
  derived server-side — which they always can here (`ProjectIDsForUser`
  from the authenticated identity). Adding a `{"type":"subscribe",
  "project_id":...}` message would have introduced exactly the
  "Client-supplied resource ID" trust problem `docs/ARCHITECTURE.md`
  "Event Authorization" warns against, for no actual benefit in this
  checkpoint's scope.
- **The user whose presence changed is excluded from their own broadcast.**
  Discovered as a real bug during test-writing, not designed in from the
  start: registering a Client fires `OnUserOnline` synchronously, and
  broadcasting to *all* of a Project's members (including the one that
  just connected) delivered a `presence.updated` about themselves onto
  their own brand-new connection — racing against (and in one test,
  arriving before) their own `connected` ack and `presence.snapshot`. Since
  a user always already knows their own connection state, skipping self in
  `presence.broadcastChange` is both correct and simpler than trying to
  sequence events instead.
- **`realtime.Hub` and `presence.Service` are separate packages with a
  narrow interface between them (`presence.ProjectMembership`,
  `presence.Broadcaster`)**, rather than one combined package. `Hub` has
  zero knowledge of Projects, users' membership, or what an "online" event
  even means to a consumer — it only knows "deliver this envelope to this
  user's live connections" and "tell me about 0↔1 connection-count
  transitions." This is what the checkpoint instructions meant by "avoid a
  one-off design that would make Tasks/Project Chat impossible to add
  cleanly later": a future Chat feature reuses `Hub` unchanged and defines
  its own event types and its own authorization-boundary interface, the
  same way `presence` did.
- **`AuthTimeout` is a `Handlers` field, not a package-level `var`.** First
  implemented as a mutable package var so a test could shorten it; `go
  test -race` immediately (and correctly) flagged a genuine race between
  one test's cleanup writing it back and another test's still-finishing
  server goroutine reading it. Moved to a field set once per `Handlers`
  instance (each test constructs its own), which is both race-free and
  arguably better design (no shared global mutable test seam at all).
- **Immediate close-on-logout *and* a periodic sweep, not just one.** Logout
  knows the exact token being revoked, so it can close matching WebSocket
  connections the instant it happens — but nothing else that can invalidate
  a session (ordinary TTL expiry, most notably) generates an event the
  real-time layer could react to immediately. The 60-second `Hub.Sweep`
  fallback is the bounded-delay mechanism for everything logout doesn't
  cover; 60s was chosen to sit comfortably under the 60s/54s pong-wait/
  ping-period heartbeat pair without meaningfully increasing Server load
  for the target team sizes.
- **`Client.Enqueue` drops (disconnects) rather than blocks or grows an
  unbounded queue** when a connection's 32-slot outbound buffer is full.
  A single slow or broken Client must never be able to stall a broadcast
  to every other connected user, and an unbounded queue for a Client that
  stopped reading is a memory-growth vector — both explicitly called out
  in the checkpoint's security-review checklist.
- **No server-side WebSocket connection/reconnect rate limiting.** The
  Client's exponential backoff (1s → 30s, with jitter) prevents any *single*
  Client from hammering the Server with reconnect attempts, which covers
  the realistic failure mode for this checkpoint's target deployment size
  (a handful of known team members). A malicious or many-Client scenario
  could still open many connections; this is accepted as a known, minor,
  documented gap rather than building connection-attempt rate limiting that
  nothing in the current threat model actually requires yet.
- **Presence events don't use a proper WebSocket close frame on
  server-initiated teardown** (session sweep, logout-driven close, Server
  shutdown all just call the underlying `conn.Close()`). Browsers report
  this as an abnormal closure (code 1006) rather than a clean 1000/1001.
  Functionally harmless — the Client's `onclose` handler treats every
  closure the same way regardless of code — but noted as a minor protocol
  politeness gap rather than silently claimed as fully "graceful."
- **The Client stops reconnecting on an `error` envelope instead of
  retrying.** Discovered via live testing: forcibly closing a session's
  WebSocket (logout, or the periodic sweep) without this would otherwise
  leave *other* open connections sharing that same revoked token (e.g. a
  second tab) in an infinite reconnect-with-backoff loop against a token
  that will never become valid again. An `error` envelope specifically
  means "the Server rejected this session," which is exactly the same
  condition the rest of the Client already treats as sign-out over HTTP
  (`apiClient.isSessionExpired`) — so `RealtimeClient` now calls an
  `onAuthError` callback that `PresenceProvider` wires to the same
  `onSessionExpired` navigation used elsewhere, and permanently stops that
  client instance rather than scheduling another retry.

**Previous checkpoints, kept below for reference — all still accurate, none
revisited this session:**

- **View-only, no member management action, by explicit project-owner
  choice** (see "Scope decision" under Completed above). "Removing a
  Project Member Experience" is real docs/UX.md scope, just not this
  checkpoint's.
- **Presence indicators (`●`/`○`) from the UX mockup were dropped entirely
  rather than rendered as a static/neutral icon** in the Members
  checkpoint — since superseded by real presence dots in this checkpoint.
- **Overview's Members section was edited** even though the Members
  checkpoint's brief was "Members," not "Overview" — justified because it
  was duplicating the exact same per-member role list the new Members
  screen now owns.
- **No new endpoint for the Members screen** in that checkpoint — reused
  `GET /api/v1/projects/{id}`.
- **`project.ValidationError` duplicates `auth.ValidationError`'s shape
  instead of importing it.**
- **`project_members.role` is `TEXT CHECK (...)`, not a Postgres `ENUM`
  type.**
- **Non-member access and "project doesn't exist" both return the same 404
  `not_found`.**
- **Malformed project IDs (not valid UUIDs) also collapse to 404.**
- **Project creation always takes the "Set Up Later" repository path,
  client-side and server-side.**
- **`ServerHome`'s Log Out button calls the real logout endpoint but
  proceeds to the Login screen even if that call fails.**
- **No nav bar for Friends/DMs/Notifications/Profile on Server Home.**

## Known Issues / Blockers

- **Work Status, Current Branch, and Current Task are still not
  implemented** (Presence itself now is). `docs/ARCHITECTURE.md`
  "Presence and Work Status Architecture" keeps these as separate, larger
  pieces of work — Work Status needs its own activity-detection design and
  explicit user Start/Stop controls; Current Branch needs the Tauri native
  layer to read local Git state; Current Task needs the Tasks feature to
  exist at all. Members/Member Detail now say so narrowly and honestly
  rather than lumping them in with "presence isn't implemented," which is
  no longer true.
- **"Removing a Project Member Experience" is still not implemented**
  (unchanged from the Members checkpoint).
- **Join Project (invite links/codes) is still not implemented.** This
  remains the reason multi-member verification requires manual test data —
  see the live verification notes above for exactly what was done and why
  it's safe (temporary Postgres row, no code/schema change).
- **No server-side WebSocket connection-attempt rate limiting** (see
  Implementation Decisions above) — accepted gap for current target scale.
- **WebSocket server-initiated closes don't send a graceful close frame**
  (see Implementation Decisions above) — cosmetic close-code gap only.
- Session token is still not persisted across app restarts (unchanged,
  deferred as before) — this also means the real-time connection has
  nothing to reconnect *to* after an actual app restart until that lands;
  reconnect-after-*temporary-disconnect-while-the-app-stays-open* is what
  this checkpoint implements and verified live.
- No automated test suite exists for the client (still unaddressed, same
  gap noted in every previous checkpoint) — this checkpoint's client-side
  verification is therefore build-correctness (`tsc`) plus the live
  two-tab browser walkthrough above, not automated tests.
- The dev Postgres volume from previous checkpoints (`korn_<timestamp>` /
  `meran_<timestamp>`, "KMJG Hub Development" / "Game Center") was **not
  used or touched this session** — this session's environment had no
  Docker available (see Tests section) and used a separate, freshly
  initialized local PostgreSQL instance instead, with its own throwaway
  test accounts. Both may exist in different environments; neither is
  shared/production data.

## Next Steps

1. **Removing a Project Member Experience** (`docs/UX.md`) — needs:
   `DELETE /api/v1/projects/{id}/members/{userId}` (or similar) with
   server-side role authorization, confirmation UI, and correct 403/404
   handling. Unblocked and unrelated to this checkpoint; can proceed
   independently.
2. **Project Chat** is now meaningfully more tractable than before this
   checkpoint: the authenticated WebSocket connection, the typed envelope
   protocol (`{v, type, data}`), and the `realtime.Hub` connection registry
   all already exist and were deliberately kept generic enough to carry a
   `chat.message.created`-shaped event without redesigning `internal/realtime`.
   The authorization pattern presence established (a narrow
   `ProjectMembership`-style interface deriving the audience server-side,
   never trusting a Client-supplied Project ID) should be reused directly.
3. **Tasks** remains self-contained and HTTP-API-shaped like Projects was —
   doesn't strictly need the real-time layer to be useful, though task
   assignment notifications could use it once it exists.
4. **Work Status / Current Branch / Current Task** is the natural
   continuation of this checkpoint specifically (same architecture
   section, same UX sections in Members/Profile) but is a genuinely
   separate design problem (automatic activity detection rules, manual
   override precedence, Tauri native Git integration for branch detection)
   and was explicitly out of scope here.
5. Session persistence (SQLite via Tauri + OS credential storage) is still
   deferred — bundle it with "Saved Servers" persistence when the Tauri
   native layer work starts, as noted in every previous checkpoint.
6. Confirm with the project owner before starting any of the above — this
   PROGRESS.md shouldn't be the thing deciding product sequencing.
