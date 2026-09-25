## v1 Feature-Complete: Phases 1-11 + Security Fix (2026-09-26)

This checkpoint closes out the entire remaining `docs/PRD.md` v1 scope. The
previous checkpoint (below) found the implemented slice was "a subset of the
PRD v1 scope — the 'Server-level social' half of the product has not been
started." This checkpoint implements all of it, in 11 sequential phases plus
one cross-cutting security fix, each independently committed, tested
(`go build`, `go vet`, `gofmt -l`, `go test`, `go test -race`, `npm run
build`, `npx vitest run`, and `cargo check` where Tauri was touched), and
verified against a real PostgreSQL instance via opt-in integration tests
before moving to the next phase.

**Commits, in order:**

1. `b3a3ef7` — **User Profiles**: display name/avatar/bio, field-level
   privacy (7 fields × 5 audiences), offline hides live fields.
2. `f992132` — **Friends & Blocking**: requests, symmetric friendships,
   blocking removes friendship and disables DMs/requests both ways.
3. `6d30125` — **Direct Messages**: access requires friends OR shared
   Project (re-checked every send), 30-day soft-delete retention matching
   Project Chat.
4. `35c4958` — **Notifications**: persistent + real-time, wired into task
   assignment, task comments, DMs, friend requests, and invitations;
   creation is best-effort and never blocks the primary action.
5. `87142a2` — **GitHub OAuth + Repository Integration**: signed
   state-token OAuth flow (no server-side state storage), AES-256-GCM
   token encryption, one repository per Project, webhook with HMAC
   signature verification, config-gated (`KMJG_GITHUB_*` env vars).
6. `4c10484` — **Direct File Transfer**: explicit accept/decline before
   any bytes reach server storage; per-file limit only, excluded from
   Project storage quota. Verified Project Chat attachment upload/download
   (already existed from an earlier checkpoint) with a new HTTP round-trip
   test.
7. `cb93a07` — **Developer Tools Launcher** (Tauri): Terminal/VS
   Code/Codex/Claude Code/OpenCode, resolves the project path only from
   the Client's own locally-stored `project_repositories` table (never an
   arbitrary path from the frontend), argument-array process spawning
   (never shell string interpolation).
8. `6fe05ef` — **Project Lifecycle**: ownership transfer (exactly one
   Owner, enforced by a unique index and verified under concurrent
   transfer in an integration test), Owner-only role promotion/demotion,
   Leave Project (Owner blocked until transfer), 30-day soft-delete +
   restore, hourly permanent-purge sweep. Kept the new lifecycle methods
   on a separate `LifecycleRepository` interface specifically so the 11
   existing test fakes implementing `project.Repository` elsewhere in the
   codebase did not need to change.
9. `9fea7e6` — **Git & Files sidebar sections**: configured Git pushes now
   post to Project Chat as a distinct non-deletable `kind='git'` system
   message (required a small schema change, migration `0016`, to make
   `author_user_id` nullable); new `chat.ListAttachments` backs a
   project-wide Files browser.
10. `94242d7` — **Multi-Server Support**: "Your Servers" list shown on
    launch instead of auto-selecting one saved Server; "Switch Server"
    from Server Home does not log out or invalidate the currently active
    Server's session, per `docs/PRD.md` "without being required to log
    out ... first."
11. `da5a35d` — **Client-Side Offline Cache**: IndexedDB-backed cache for
    the three highest-value screens (Server Home project list, Project
    Chat history, Project detail), namespaced per Server address so one
    saved Server's cache can never leak into another's; connection status
    is derived from the existing WebSocket client rather than a separate
    `navigator.onLine` signal. Deliberately does not queue offline
    message composition — `docs/PRD.md` "Offline Behavior" requires
    connectivity for that, so there is nothing to queue.
12. `3d45013` — **Security fix, found during this checkpoint's own
    closing review**: Phase 8's soft-deletion correctly excluded deleted
    Projects from `project.Repository`'s own queries, but every other
    package that authorized access via a plain `project_members`
    membership check — Chat, Tasks, Work Context, Invitations, and (found
    during the fix, not in the original list) Direct Messages, Direct
    File Transfer, and `profile.Service`'s "shared Project" privacy
    check — did not also verify the Project wasn't deleted. A member of a
    Project at the time of its deletion could keep reading and writing
    its Chat/Tasks/Work Context and use its invitations for the full
    30-day retention window via direct API calls, even though the Client
    UI could no longer reach the Project. Fixed across ~15 queries in
    `chat.go`, `tasks.go`, `workcontext.go`, `invitations.go`,
    `directmessage.go`, `filetransfer.go`, and `profile.go`'s
    `ShareProject`; proven with a new opt-in integration test
    (`deleted_project_access_test.go`) that fails on the pre-fix SQL and
    passes after.

**What this means against `docs/PRD.md`:** every v1 feature area listed in
that document now has server-side, client-side, and test coverage. Known,
explicitly out-of-scope-for-v1 simplifications made along the way (not
gaps against the PRD, which doesn't require them): no resumable file
transfers, no offline message queueing, no conflict-resolution UI (server
state always wins on reconnect), GitHub "Sign in with GitHub" as an
*authentication* method is not built (only *linking* an existing account,
since the OAuth callback has no way to authenticate an unauthenticated
browser tab back into a Client session — this is a real architectural
constraint, not an oversight, documented in the Phase 5 commit), and
per-repository access-scoped visibility for private repos is not enforced
beyond what GitHub's own API already returns.

**Verification performed:** every phase's Go changes passed
`go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./...`, and `go
test -race ./...`; every phase's TypeScript changes passed `npm run build`
and (where tests existed) `npx vitest run`; Tauri changes (Phases 7, 9, 10)
passed `cargo check` using the project's rustup-managed toolchain at
`~/.cargo/bin` (the sandbox's system `/usr/bin/cargo` 1.75 is too old for
this repo's lockfile). Phases with new SQL authorization logic (3, 6, 8, 9,
and the closing security fix) additionally ran their opt-in PostgreSQL
integration tests against a real database before being considered done.

**Not yet done / follow-ups for a future checkpoint** (none block v1, all
were explicitly noted by name during implementation rather than silently
skipped):
- Git activity feed shows configuration state, not a scrollable history of
  past pushes; the Client doesn't yet subscribe to `project.git.*`
  real-time events.
- Repository collaborator invitations and automatic repository-access
  removal on member leave/removal (`docs/PRD.md` "Repository Collaborator
  Invitations", "Removing Project Members" Git-access clause) are not
  built.
- No server-administrator-facing recovery UI for deleted Projects beyond
  the data model supporting it (the PRD requires this exists at the
  Server-Administrator level, separate from the per-user 30-day
  self-restore already built).
- Offline cache covers 3 screens (Server Home, Project Chat, Project
  detail); Friends/DMs/Notifications/File Transfers/GitHub info have no
  offline fallback yet.
- Direct File Transfer's `expired` status exists in the schema but nothing
  transitions a transfer into it; uploaded files have no automatic
  cleanup job (they persist until downloaded and are never purged
  afterward).

---

## Live Verification, Docs Audit, and Desktop Build (2026-09-26)

This checkpoint did not add product features. It (1) proved the existing
stack actually runs end-to-end outside the container smoke test, (2) read
every file in `docs/` in full and cross-checked it against the real code to
get an honest v1 completeness picture, and (3) produced and verified real
installable desktop packages. No repository code changes were needed for any
of this except new local-dev tooling (below).

### End-to-end live run, no mocks

Earlier in this session the app was briefly demoed against a throwaway
Python mock API — that mock's response shapes did not match
`client/src/lib/apiClient.ts` (e.g. login's `identifier` field, `{user,
session}` response shape) and broke after Connect Server. That approach was
abandoned. Replaced with the real stack:

- Added `scripts/dev-up.sh` / `scripts/dev-down.sh` — committed in
  `f19cea5`. Starts a private PostgreSQL under `.dev-data/` (gitignored, no
  system Postgres or Docker needed), builds and runs the real Go server
  against it, and starts the Vite dev client on port 1420 (matches
  `tauri.conf.json`'s `devUrl`). Idempotent; safe to re-run. Fixed a real bug
  caught only by testing the stop path: `npm run dev` forks a `vite` child
  that a plain `kill` on the parent orphans holding the port — fixed with
  `setsid` + process-group kill in `dev-down.sh`.
- Verified live in the browser with real accounts (`alice` /
  `password123`, seeded via the real HTTP API, not fixtures): Connect
  Server → Login → Server Home (project list) → Overview (real presence:
  "Members Online: 1 / 1") → Chat (real seeded message from Postgres) →
  Tasks (real Kanban board, 3 seeded tasks). Screenshots saved during that
  session to `/tmp/claude-chrome-screenshots-FcnFIK/`.

### Desktop build (Tauri release bundles)

`npm run tauri build` initially failed twice against the sandbox's system
Rust (`rustc 1.75.0`, Ubuntu apt package): first on `Cargo.lock`'s `version =
4` (needs cargo ≥1.78), then — after a lockfile regeneration attempt — on a
transitive dependency requiring Rust edition 2024 (needs rustc ≥1.85). Fixed
by installing a modern toolchain via `rustup` into `~/.cargo`
(`--no-modify-path`, does not touch the system Rust or shell rc files).
**Confirmed the committed `Cargo.lock` is fine as-is** — with the modern
toolchain in `PATH`, the original unmodified lockfile builds cleanly
end-to-end (verified with a second, independent full build). No lockfile
change was committed.

Produced and verified real artifacts at
`client/src-tauri/target/release/bundle/`:
- `deb/client_0.1.0_amd64.deb` (3.7 MB, depends on `libwebkit2gtk-4.1-0`,
  `libgtk-3-0`)
- `rpm/client-0.1.0-1.x86_64.rpm` (3.7 MB)
- `appimage/client_0.1.0_amd64.AppImage` (81 MB, no install needed)

The AppImage was actually launched (not just built): the process stayed
alive, produced no error output, and `xwininfo` confirmed a real 800×600
`"client"` window on the X display — matching `tauri.conf.json`'s configured
window size. This is the first time in this project's history the desktop
bundle has been built and run, not just `cargo check`ed.

### Full docs-vs-code completeness audit

Read `docs/VISION.md`, `docs/PRD.md`, `docs/UX.md`, `docs/ARCHITECTURE.md`
in full (~5,000 lines total) and checked every documented v1 feature against
the actual `server/internal/*` packages and `client/src/features/*`
directories (not against this file's own claims). Verdict: **the
implemented slice (Auth, Projects, Chat, Tasks, Presence, Work Context,
Invitations) is genuinely solid and tests clean, but it is a subset of the
PRD v1 scope — the "Server-level social" half of the product has not been
started.**

Implemented and verified against code (not just claimed):
- Local auth (Argon2id), sessions, projects (create/list/detail), Project
  Chat (messages, realtime, soft-delete + 30-day retention, attachments),
  Tasks (Kanban, self/other assignment with accept/decline, current task,
  comments) — `internal/task/service.go` has every method the PRD's Task
  Management section describes.
  Presence over authenticated WebSocket, Work Context (manual + Tauri
  native Git branch detection), direct invitations + invite links/codes,
  and **member removal — actually wired in `Members.tsx`**, correcting an
  earlier "view-only" note further down this file.

Documented in `docs/PRD.md` / `docs/UX.md` / `docs/ARCHITECTURE.md` but
**no code exists for any of it** (verified by package/directory absence, not
just missing UI):
- GitHub OAuth login and GitHub repository integration (code comments say
  this explicitly: "GitHub authentication is out of scope for this
  checkpoint")
- Friends, Friend Requests, Blocking — no `internal/friend` package
- Direct Messages — no package, no client screen
- Notifications (Server-level, persistent) — no package
- User Profile beyond id/username/email — `internal/user/user.go`'s own
  comment says "the broader profile model (avatar, bio, presence, etc.)
  belongs to a later slice"; no field-level privacy
- Direct File Transfer (peer-style accept/decline file send, distinct from
  Project Chat attachments) — not implemented
- Git activity feed / configurable push notifications — Overview/Members
  still show literal "not implemented yet" text
- Developer Tools launcher (Terminal / VS Code / etc. from the project
  directory) — no Tauri command, no client screen at all
- Project deletion + 30-day recovery, Ownership transfer, Promote/Demote
  role — `project/service.go` only has `Create/List/GetDetail/RemoveMember`
- Multi-server support (Saved Servers list, Server switcher) —
  `ConnectServer.tsx` handles exactly one server, no saved-server list
- Client-side offline cache of messages/tasks/projects and the offline
  banner/sync UX — Client SQLite is currently used only for saved sessions
  and repo-path mapping, not the general cache `docs/ARCHITECTURE.md`
  describes
- Backup automation / Server-side backup configuration

### What "ready" means depends on the bar

- Against the full `docs/PRD.md` v1 scope: **not ready** — roughly half the
  documented v1 feature set (everything server-level/social) is unbuilt.
- As a working "project chat + tasks + presence" tool for a small team using
  the already-implemented slice: **ready** — proven end-to-end above with
  real accounts, not just unit tests.

### Next Steps

1. **Treat `docs/PRD.md`, `docs/UX.md`, and `docs/ARCHITECTURE.md` as the
   spec for all remaining work** — they are current and were re-read in
   full this session; nothing in them was found stale or contradicted by
   the codebase for the areas they cover. Do not re-derive product
   decisions (roles, retention periods, privacy audiences, etc.) from
   scratch — they're already fully specified there.
2. Suggested build order for the unimplemented half, roughly by how much
   other undone work depends on it: (a) User Profile (basic fields +
   privacy, since Friends/DMs/Notifications all reference profile-visible
   state), (b) Friends + Blocking, (c) Direct Messages, (d) Notifications,
   (e) GitHub OAuth + repository integration, (f) Direct File Transfer,
   (g) Developer Tools launcher, (h) Project deletion/recovery + ownership
   transfer + role management, (i) Multi-server client support, (j)
   client-side offline cache.
3. Put the published Server behind trusted HTTPS before using it over a
   network; configure allowed origins for the actual deployment.
4. Commit and push this release-readiness checkpoint (`f19cea5` is local
   only, matching this file's own note below about earlier local-only
   commits).

---

## Current Release Readiness (2026-09-25)

The Tasks checkpoint is committed locally in `233bb00`. The current working
tree completes the remaining release-readiness gaps requested by the project
owner: Project Work Status with manual override, Tauri-native Current Branch
detection, Project Chat attachments and cursor pagination, secure persisted
desktop sessions, automated Client tests, and deployment bootstrap/health
checks.

Current verification is clean: the full Go suite (including the opt-in
PostgreSQL integration test) and `go vet ./...` pass; Client Vitest tests and
the production TypeScript/Vite build pass; Tauri `cargo check` and native Rust
unit tests pass. `docker compose config --quiet` passes after
`scripts/setup-env.sh` creates a mode-600 ignored `.env`.

**2026-09-25 host deployment smoke test: passed.** After adding the host user
to the `docker` group and starting a new group shell, `docker compose up
--build -d` completed successfully. PostgreSQL became healthy, `kmjg-server`
started and exposed port 8080, and `curl http://localhost:8080/api/v1/health`
returned `{"status":"ok","service":"kmjg-hub-server","version":"0.1.0"}`.
The Compose log's optional Buildx/Bake warning did not affect the successful
standard Docker build.

## Resume Here (historical Tasks checkpoint snapshot)

Latest local feature commit: `b9be2b1` (`feat: complete task assignment flow`)
on `main`. It and its prerequisite `590d4bf` are **local only and have not
been pushed**. `be47289` remains the latest feature commit on `origin/main`.

The working tree is intentionally dirty with the next, uncommitted Tasks
slice. Do not discard it: it adds member Current Task data to Project detail,
renders that context on Overview and Members, adds the assign-another-member
control to Task detail, and publishes/consumes Task change events over the
authenticated WebSocket. It also adds the first filesystem storage abstraction
and deployment-configured attachment limits; all of this remains uncommitted
pending a final checkpoint commit.

Current implementation state:
- Connect Server: complete
- Local Auth / Sessions: complete
- Projects / Overview: complete
- Members view-only: complete, now with real presence dots
- **Presence / Authenticated WebSocket Foundation: complete (commit `ce108ac`)**
- **Presence Freshness / Server Lifecycle Fix: complete, committed (commit
  `bf29236`) — see "Checkpoint 4 addendum" below**
- **Removing a Project Member Experience: implementation complete and
  verified, committed and pushed in `be47289` — see Checkpoint 5 below**
- **Direct Project Invitations: implementation complete and verified,
  committed and pushed in `be47289` — see Checkpoint 6 below**
- **Invite Links and Codes: implementation complete and verified,
  committed and pushed in `be47289` — see Checkpoint 7 below**
- **Project Chat (text messages, realtime delivery, and moderation):
  implementation complete and verified, committed and pushed in `be47289` —
  see Checkpoint 8 below**

**Tasks implementation and verification are complete; the checkpoint is not
yet committed.** The real-time Task event, focused unit/HTTP coverage, and
PostgreSQL-backed Task coverage now pass. Project Chat file attachments remain a
separate storage checkpoint; the text timeline, history, realtime delivery,
message deletion, and 30-day cleanup lifecycle now work. Invitation
notifications and real-time invitation events remain separate enhancements;
persistent invitation state and all required join paths work through HTTP.

### Tasks checkpoint — implementation state

#### Committed locally (not pushed)

`590d4bf` (`feat: add project tasks foundation`) adds the persistent Server model and HTTP
surface for Tasks: project-scoped Kanban statuses, task creation/listing and
detail retrieval, status updates, self-assignment, explicit other-member
assignment requests with accept/decline, one current task per user, and
persistent task comments. Migration `0006_tasks.sql` creates the required
tables. Every data operation derives access from authoritative
`project_members` rows; client-supplied project/task IDs never grant access.

`b9be2b1` (`feat: complete task assignment flow`) enables **Tasks** in Project
Workspace and adds a Kanban board, task create/status moves, task detail,
comments, self-assignment, current-task action, incoming assignment inbox,
and accept/decline flow. Both commits are local checkpoints only.

#### Uncommitted work currently present

- Project detail now includes each member's Current Task title only when it
  belongs to that Project; it is rendered on Overview and the Members list.
- Task detail provides an assign-to-another-member selector. Assigning another
  member creates a persistent request; their Tasks inbox accepts or declines
  it. Self-assignment still applies immediately.
- Every successful Task mutation (creation, status, self-assignment,
  assignment request/response, Current Task selection, or comment) broadcasts
  `project.task.changed` to the authoritative current Project members. The
  event contains only Project/Task IDs; Client reloads its HTTP-authorized
  board, inbox, and Project detail, so it never applies stale or unauthorized
  Task payloads from WebSocket state.
- `internal/task/service_test.go` covers mutation-after-persistence event
  delivery and failed-mutation non-delivery. `internal/httpapi/tasks_test.go`
  covers Task endpoint authentication, validation, and response DTOs.
- `internal/store/postgres/tasks_integration_test.go` is opt-in through
  `KMJG_TEST_DATABASE_URL` and refuses to run destructive setup unless the
  current database name contains `test`. It exercises migrations, non-member
  Task authorization, assignment-response authorization and acceptance, and
  Current Task's transactional auto-transition/replacement rules against live
  PostgreSQL.
- Task creation now exposes the optional Due Date field required by the PRD.
- `internal/storage` provides a tested opaque-ID filesystem store with atomic
  no-overwrite publication. `KMJG_FILE_STORAGE_DIR`,
  `KMJG_MAX_UPLOAD_BYTES`, and `KMJG_MAX_PROJECT_STORAGE_BYTES` are now
  deployment-configurable. This is foundation only: it is not yet wired into
  Project Chat metadata/upload/download APIs.

#### Still required before committing the Tasks checkpoint

- Review the combined uncommitted diff and commit the final checkpoint. Do not
  push unless the user explicitly asks.

#### Verification performed so far

- `npm run build` passed after the task-detail, assignment-inbox, and
  assign-member UI work.
- `go test ./...` passed after adding the task assignment inbox endpoint and
  after the first Current Task Project-detail query implementation.
- After the real-time slice, focused `go test ./internal/task ./internal/httpapi`
  and `npm run build` passed. The full Server suite must be re-run immediately
  before the final commit.
- **2026-09-25 verification:** a dedicated local PostgreSQL 16 database
  (`kmjg_hub_test`) was provisioned and the opt-in Task integration test
  passed. `KMJG_TEST_DATABASE_URL=postgres://kmjg_test:kmjg_test@127.0.0.1:5432/kmjg_hub_test?sslmode=disable`
  plus `go test -count=1 ./...` passed for the full Server suite. `npm run
  build` passed for the Client. Tauri `cargo check` passed with project-local
  Rust stable 1.98.1 after installing the Ubuntu native build dependencies.
- Go 1.23.1 is installed locally under `.tools/go`; module/build caches are
  under `.cache`; both paths are intentionally Git-ignored. Use
  `GOMODCACHE="$PWD/../.cache/go-mod" GOCACHE="$PWD/../.cache/go-build"
  ../.tools/go/bin/go test ./...` from `server/`.

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
  to), and `App.Close()` closes every open WebSocket connection before
  closing the database pool. **This checkpoint's original `Close()` had a
  lifecycle gap fixed in the follow-up session below — see "Checkpoint 4
  addendum" for the corrected design (App now owns its own cancellable
  context and waits for its background goroutines before releasing the
  pool).**

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
  whole authenticated app session. Originally exposed
  `useProjectPresence(projectId)` returning `{ status, isOnline(userId) }`,
  gated purely on the connection being `"open"`. **This gating had a
  reconnect staleness gap, fixed in the follow-up session below — the hook
  now also returns a per-Project `ready` flag, and `isOnline` is gated on
  that instead of on `status` alone; see "Checkpoint 4 addendum."**
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
  implemented yet" note is gone. Originally gated the real `Members Online:
  N / M` count (`docs/UX.md` "Project Overview": "Members Online \n 3 / 4")
  purely on the real-time connection being `"open"`, falling back to the
  member count + a "Connecting to real-time presence…" note otherwise.
  **That gating had a reconnect-staleness bug, fixed in the follow-up
  session below — see "Checkpoint 4 addendum."**
- **`src/features/projects/Members.tsx`** + **`Members.css`** — each member
  row shows a real presence dot (green `●` online / hollow `○` offline);
  originally gated on the connection being `"open"`, **since corrected to
  gate on fresh per-Project presence — see "Checkpoint 4 addendum" below**.
  Before presence is known, no dot is drawn and a neutral note explains
  why. The Member Detail overlay's blanket "Presence, work status, current
  branch, and current task are not implemented yet" note was narrowed to
  "Work status, current branch, and current task are not implemented yet"
  (Presence itself is real now) and gained an explicit `"● Online"` /
  `"○ Offline"` / `"Presence unknown"` line (wording also adjusted in the
  follow-up session — see below). The four quick actions (Chat, Send File,
  View Branch, View Current Task) remain disabled — untouched, still out of
  scope.

### Checkpoint 4 addendum: Presence Freshness / Server Lifecycle Fix (commit `bf29236`)

**Scope:** a follow-up review of the Presence / Authenticated WebSocket
Foundation checkpoint above found and fixed two confirmed bugs — one
server-side (App/Hub background-goroutine shutdown ordering), one
client-side (stale presence surviving a reconnect). No Tasks/Chat/Work
Status/Git/invitations work was touched; scope was deliberately kept to
these two fixes.

**Server — background-goroutine lifecycle fix:**

`App.New(ctx, cfg)` started the session-sweep goroutine and the
`realtime.Hub` using the `ctx` passed in from `cmd/server/main.go` (the
process's signal-derived context), and `App.Close()` called
`Realtime.Shutdown()` + `Pool.Close()` without stopping or waiting for
those goroutines first. On a non-signal shutdown path (e.g. the HTTP server
failing to start), that `ctx` is never cancelled, and `main.go`'s
`defer kmjgApp.Close()` runs *before* `defer stop()` regardless — so
`Close()` could not rely on `ctx` being done, and could close the database
pool while `runSessionSweep` or the Hub's internal presence-callback
dispatcher still had live queries in flight.

Fixed by giving `App` its own child context it fully owns:
- `App` now derives `appCtx, cancel := context.WithCancel(ctx)` in `New`,
  and uses `appCtx` (not the raw `ctx`) for `realtime.NewHub` and the
  session-sweep goroutine.
- `App` gained `cancel context.CancelFunc`, `wg sync.WaitGroup` (tracks the
  sweep goroutine), and `closeOnce sync.Once` (idempotency).
- `realtime.Hub` gained a `Wait()` method, backed by a `done` channel closed
  when its internal transition-dispatch goroutine exits, so a caller that
  cancels the Hub's context can also confirm that goroutine has actually
  stopped (not just been asked to).
- `App.Close()` is now: `cancel()` → `Realtime.Shutdown()` → `wg.Wait()` →
  `Realtime.Wait()` → `Pool.Close()` — every App-owned background goroutine
  is stopped *and joined* before the pool closes, unconditionally, and
  `Close()` is safe to call more than once.

Files: `server/internal/app/app.go`, `server/internal/realtime/hub.go`.
Tests added: `server/internal/app/app_test.go` (new —
`TestRunSessionSweepExitsOnContextCancellation`, using a fake, DB-free
`session.Repository`) and `server/internal/realtime/hub_test.go`
(`TestWaitBlocksUntilTransitionDispatcherExits`). No DB-backed test covers
the full `New`→`Close` path — the repo has no Postgres test harness — so
these two tests isolate and cover the exact synchronization primitives
`Close` depends on instead.

Validation: `gofmt -l .` clean; `go build ./...`, `go test ./...`,
`go test -race ./...`, `go vet ./...` all clean; `git diff --check` clean.

**Client — presence freshness-after-reconnect fix:**

`RealtimeClient` marks the connection `"open"` on the Server's `"connected"`
event, which always arrives *before* that connection's per-Project
`presence.snapshot` (the Server enqueues them in that order). Meanwhile
`PresenceProvider` never cleared its `projects` presence cache across a
reconnect. Net effect: right after a reconnect, `status` was already
`"open"` but `Overview`'s `presence.status === "open" ? count : null` logic
had no way to distinguish "fresh data" from "stale-from-before-the-drop" or
"no data yet" — briefly showing a wrong "Members Online: 0/N" (or a
lingering stale count) until the real snapshot for that Project arrived.

Fixed with a per-Project readiness signal rather than overloading
Online/Offline:
- `PresenceState` gained `readyProjects: Record<projectId, boolean>`, set
  `true` **only** inside the `presence.snapshot` handler for that Project —
  never by `"connected"` and never by a `presence.updated` event.
- `onConnectionStatusChange` now clears both `projects` and `readyProjects`
  whenever the status moves away from `"open"` (reconnect or terminal
  close), so a previous connection's presence can never leak into whatever
  connection replaces it. Normal navigation with the same live connection
  triggers no status change, so a valid snapshot is never discarded by this.
- `useProjectPresence` now returns `ready = status === "open" &&
  readyProjects[projectId]` alongside `status`/`isOnline`; `isOnline` is
  gated on `ready`, not on `status` alone.
- `Overview.tsx` gates `onlineCount` on `presence.ready` (was
  `presence.status === "open"`) — no more "0/N" merely because the socket
  reopened before the Project's own snapshot arrived.
- `Members.tsx`'s "Connecting to real-time presence…" note and
  `PresenceDot` now gate on `presence.ready`; `MemberDetail` always reads
  `presence.isOnline(...)` directly (it already returns `undefined` unless
  `ready`) and shows "Presence unknown" (text simplified — it's no longer
  only shown while reconnecting; it now also covers "open but not yet
  synced").

Reviewed (no change made): `RealtimeClient`'s blanket "any `error` envelope
⇒ auth error, stop reconnecting" handling. Confirmed via the Go server code
that `writeWebSocketError` is only ever called with code `"unauthorized"`,
only from pre-registration `authenticateWebSocket` — the current protocol
cannot emit a non-auth `"error"`, so the existing handling is safe as-is.

Files: `client/src/features/presence/PresenceProvider.tsx`,
`client/src/features/projects/Overview.tsx`,
`client/src/features/projects/Members.tsx`. No server files, no
`realtimeClient.ts` changes.

Tests added: none. The `client` package has no test runner configured at
all (no `test` script, no Vitest/Jest, no `@testing-library/*`) — adding
one was judged out of scope for this focused fix. Verified instead by code
inspection against the four required scenarios (old snapshot not exposed
after reconnect; `"connected"` before a fresh snapshot exposes nothing;
a fresh snapshot restores known presence; Overview never turns Unknown into
"0 online") plus `npm run build` (`tsc && vite build`), which passed clean.
This client-side automated-test gap is the same pre-existing gap already
tracked under "Known Issues / Blockers" below.

Validation: `npm run build` clean; `git diff --check` clean; no Go tests
run for this half (no server/protocol files changed).

**Committed and pushed** as `bf29236` (`fix: harden realtime presence
lifecycle`) on `main`; see "Resume Here" / Git status.

### Checkpoint 5: Removing a Project Member Experience (commit `be47289`)

**Scope:** implement the KMJG Hub membership-removal portion of
`docs/UX.md` "Removing a Project Member Experience". Repository access is
explicitly a separate external Git-provider operation; no Git provider or
repository model exists yet, so the UI says this removal affects KMJG Hub
membership only.

**Server:**

- Added authenticated `DELETE /api/v1/projects/{id}/members/{userID}`.
- `project.Service.RemoveMember` implements the documented role policy:
  Owner may remove Admin or Member; Admin may remove Member only; no role may
  remove the Owner. Self-removal is rejected so it cannot bypass the separate
  Leave Project / ownership-transfer rules.
- PostgreSQL performs the same role check in the `DELETE ... USING ...`
  statement, making the authorization decision authoritative at deletion time
  rather than trusting an earlier service-layer read.
- The HTTP contract returns `204` on success, `403` for forbidden/self-removal,
  and `404` for a non-accessible Project or missing member.

**Client:**

- Member Detail exposes `Remove Member` only for actions allowed by the
  current viewer role; Server authorization remains the security boundary.
- A confirmation step names the target member and explains that no repository
  access is changed because Git integration is not configured.
- On success, the roster updates locally without a second fetch.

**Tests and verification:**

- Added HTTP coverage for Owner/Admin/Member removal rules and for a removed
  member losing Project access.
- `go test ./...`, `go test -race ./...`, `go build ./...`, `go vet ./...`,
  `gofmt -l internal`, `npm run build`, and `git diff --check` all pass.
- During verification, `TestWebSocketRejectsOversizedMessage` exposed a
  scheduling-sensitive assertion: a server enforcing its read limit may close
  the TCP connection before the client completes its oversized write. The test
  now correctly accepts that peer-close result as rejection, and full/race
  suites pass after the adjustment.
- There is still no PostgreSQL integration-test harness in this repo. The
  parameterized deletion SQL compiles with the Server, but has not run against
  a live PostgreSQL instance in this checkpoint.

**Changed files:** `server/internal/project/{project.go,service.go,service_test.go}`,
`server/internal/store/postgres/projects.go`,
`server/internal/httpapi/{server.go,projects.go,projects_test.go,websocket_test.go}`,
and `client/src/{App.tsx,lib/apiClient.ts,features/projects/Members.tsx,features/projects/Members.css,features/projects/ProjectWorkspace.tsx}`.

### Checkpoint 6: Direct Project Invitations (commit `be47289`)

**Scope:** implement Direct Invitations to an existing account on the same
KMJG Hub Server. Invite Links/Codes, email delivery, notifications, and Git
repository invitations are explicitly separate features.

**Server and persistence:**

- Migration `0003_project_invitations.sql` adds authoritative invitation
  state, optional expiration, and mutually exclusive accepted/declined/
  cancelled terminal timestamps.
- New `internal/invitation` domain service validates the supported expiration
  choices (`1h`, `1d`, `7d`, `30d`, `never`) and owns transitions.
- Owner/Admin can invite an existing username or email; regular Members
  receive `403`. Self-invites, existing members, and duplicate active invites
  receive `409`; unknown recipients receive `404`.
- Recipient-only accept/decline and original-inviter-only cancellation are
  enforced by PostgreSQL predicates. Accepting inserts Member membership and
  marks the invitation accepted in one transaction.
- Creation takes a Project/recipient advisory transaction lock, preventing
  concurrent requests from both creating active invitations. Acceptance locks
  the invitation row before checking/inserting membership.
- API: create/list/cancel under `/api/v1/projects/{id}/invitations`; received
  list and accept/decline under `/api/v1/invitations`.

**Client:**

- Owner/Admin Members screen has an invite form for username/email, all five
  expiration choices, active-invitation list, and cancellation.
- Server Home shows received Direct Invitations with Decline and Accept.
  Accepting opens the newly joined Project immediately.
- The confirmed default expiration for a new Direct Invitation is 7 days.

**Tests and verification:**

- Domain tests cover every expiration choice and invalid expiration.
- HTTP tests cover role denial, create/list, wrong-recipient denial, accept
  creating usable membership, decline, inviter cancellation, and cancellation
  denial for another user.
- `go test ./...`, `go test -race ./...`, `go build ./...`, `go vet ./...`,
  `gofmt -l internal`, `npm run build`, and `git diff --check` pass.
- This environment has no Docker, Podman, `psql`, or `postgres` binary, so the
  new migration/queries could not be executed against live PostgreSQL. The
  persistence layer is compile-verified; live PostgreSQL remains an explicit
  verification gap, not silently claimed as passed.

**Changed files:** `server/internal/invitation/*`,
`server/internal/store/postgres/{invitations.go,migrations/0003_project_invitations.sql}`,
`server/internal/httpapi/{invitations.go,invitations_test.go,server.go,auth_test.go}`,
`server/internal/app/app.go`, `client/src/lib/apiClient.ts`,
`client/src/features/server-home/ServerHome.tsx`, and the Members UI files.

### Checkpoint 7: Invite Links and Codes (commit `be47289`)

**Scope:** add shareable Project invite credentials that an authenticated user
can intentionally submit from Server Home to join immediately as Member. A
single secure credential can be copied as either its code or the displayed
link; both resolve to the same Server-authoritative resource.

**Security and persistence:**

- Migration `0004_project_invite_credentials.sql` stores Project/creator,
  optional expiration, optional maximum uses, current use count, revocation,
  and a unique SHA-256 token hash. Raw invite secrets are never persisted.
- Secrets are 32 random bytes from `crypto/rand`, encoded URL-safe. The raw
  code is returned only on creation; list responses cannot recover or expose
  it.
- Owner/Admin can create, list, and revoke credentials. Members receive `403`.
- Credential consumption locks the credential row, rechecks revoked/expired/
  exhausted state, rejects existing members, inserts membership, and increments
  the use count in one transaction. This prevents concurrent joins from
  exceeding `max_uses`.
- Specific API errors distinguish invalid, expired, revoked, exhausted, and
  already-member cases.

**API and Client:**

- Project endpoints create/list/revoke under
  `/api/v1/projects/{id}/invite-credentials`; authenticated join is
  `POST /api/v1/invitations/join` with either a raw code or displayed link.
- Members UI lets Owner/Admin choose expiration, positive max uses or
  unlimited uses, create a code/link, view active use counts, and revoke.
- Server Home's previously disabled Join Project control is now a real
  link/code input. Successful join opens the Project immediately.
- Confirmed UI defaults are 7-day expiration and one use.

**Tests and verification:**

- Domain tests verify cryptographic secret generation, hash-only persistence,
  URL parsing, supported expiration choices, and invalid use limits.
- HTTP tests cover manager authorization, invalid limits, raw-secret omission
  from lists, already-member rejection, link consumption, maximum-use
  exhaustion, revocation, and post-join Project access.
- `go test ./...`, `go test -race ./...`, `go build ./...`, `go vet ./...`,
  `gofmt -l internal`, `npm run build`, and `git diff --check` are the required
  final gates for this combined working tree.
- As in Checkpoint 6, this environment has no PostgreSQL/container runtime;
  live migration/query execution remains unavailable and must be run in an
  environment with PostgreSQL before deployment.

**Changed files:** `server/internal/invitation/*`,
`server/internal/store/postgres/{invitations.go,migrations/0004_project_invite_credentials.sql}`,
`server/internal/httpapi/{invitations.go,invitations_test.go,server.go}`,
`client/src/lib/apiClient.ts`, `client/src/features/projects/Members.tsx`,
`client/src/features/projects/Members.css`, and
`client/src/features/server-home/ServerHome.tsx`.

### Checkpoint 8: Project Chat — Text, Realtime, and Moderation (commit `be47289`)

**Scope:** implement the usable text-message core of the one primary Project
Chat specified for v1: persistent recent history, sending, authorized realtime
delivery, and soft-deletion/moderation. Project Chat file attachments are
explicitly not included because no Server storage backend or operator upload
limits exist yet; the Client labels that limitation instead of presenting a
non-functional attachment control.

**Server and persistence:**

- Migration `0005_project_chat.sql` adds Project-scoped, user-authored messages,
  a bounded 4,000-character body, creation metadata, and soft-deletion metadata.
  Normal history queries never return soft-deleted content.
- New `internal/chat` domain service validates and persists a message before
  publishing `project.message.created`. It derives every realtime recipient
  from current Server-owned Project membership; a Client cannot choose the
  broadcast audience.
- Authenticated endpoints under
  `/api/v1/projects/{id}/chat/messages` list the latest 50 active messages,
  create one message, and delete one message.
- Create and history access require current membership. The PostgreSQL create
  statement makes the membership check part of the insert, and the history
  query rechecks membership while reading so a concurrent removal cannot expose
  Project messages.
- A sender may delete their own message. Owner/Admin may delete another user's
  Project Chat message for moderation. Regular Members receive `403`; missing,
  inaccessible, malformed, and already-deleted resources collapse to `404`.
  PostgreSQL performs role authorization and soft deletion in one statement.
- Successful deletion publishes `project.message.deleted` only to current
  members. An App-owned retention sweep runs at startup and hourly, permanently
  removing messages whose documented 30-day soft-deletion window has elapsed;
  shutdown cancels and waits for it before closing the database pool.

**Client:**

- Chat is now an enabled Project Workspace section with a scrollable timeline,
  empty/loading/error states, author and local timestamp, multiline composer,
  Enter-to-send (Shift+Enter for newline), and a Unicode-code-point counter that
  matches Server validation.
- HTTP history is authoritative after opening/reconnecting. Typed WebSocket
  create/delete events update the open timeline immediately; message IDs are
  deduplicated so the sender cannot get a duplicate from its HTTP response and
  its own realtime event.
- Delete actions appear only for the message author or Owner/Admin and require
  the confirmation described by `docs/UX.md`; the Server independently enforces
  the real permission.
- The realtime provider bounds its in-memory Chat event buffer to 100 events and
  clears it across connection generations. Missed events are repaired by HTTP
  history rather than treating WebSocket delivery as authoritative storage.

**Tests and verification:**

- Domain tests cover trimming/validation, exact 4,000-code-point Unicode input,
  persist-before-publish failure behavior, bounded history retrieval, delete
  publish behavior, and the 30-day purge cutoff.
- HTTP/integration tests cover send/list, non-member denial, empty-message
  rejection, Member-vs-Owner deletion permissions, deleted-content hiding, and
  real WebSocket create/delete delivery to a current member without leaking the
  create event to an outsider.
- `go test -race ./...`, `go build ./...`, `go vet ./...`, `gofmt -l internal`,
  `npm run build`, and `git diff --check` all pass. Project Chat domain/HTTP
  tests also passed 20 consecutive runs.
- This environment has no Docker, Podman, `psql`, or PostgreSQL server binary.
  Migration `0005` and the PostgreSQL Chat queries are compile-verified but have
  not been executed against a live PostgreSQL instance. This remains required
  before deployment.

**Confirmed product values:** Project Chat accepts up to 4,000 characters,
loads the latest 50 messages initially, and runs deleted-message retention
cleanup at startup plus hourly. These are no longer tracked as open
assumptions.

**Changed files:** `server/internal/chat/*`,
`server/internal/store/postgres/{chat.go,migrations/0005_project_chat.sql}`,
`server/internal/httpapi/{chat.go,chat_test.go,server.go,auth_test.go}`,
`server/internal/app/app.go`, `client/src/lib/{apiClient.ts,realtimeClient.ts}`,
`client/src/features/presence/PresenceProvider.tsx`, and
`client/src/features/projects/{ProjectChat.tsx,ProjectChat.css,ProjectWorkspace.tsx,ProjectWorkspace.css}`.

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
- **The Presence / Authenticated WebSocket Foundation checkpoint (commit
  `ce108ac`) and the Checkpoint 4 addendum (Presence Freshness / Server
  Lifecycle Fix, commit `bf29236`) are both implemented, fully verified,
  committed, and pushed to `origin/main`.**

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

**Checkpoint 4 addendum (Presence Freshness / Server Lifecycle Fix, commit
`bf29236`):**

Server, new:
- `server/internal/app/app_test.go` —
  `TestRunSessionSweepExitsOnContextCancellation`.

Server, modified:
- `server/internal/app/app.go` — App-owned `appCtx`/`cancel`, `wg`,
  `closeOnce`; corrected `Close()` ordering (see addendum above). Note this
  is on top of, and supersedes the lifecycle description under this same
  file in the checkpoint entry above.
- `server/internal/realtime/hub.go` — added `done` channel + `Wait()`.
- `server/internal/realtime/hub_test.go` —
  `TestWaitBlocksUntilTransitionDispatcherExits`.

Client, modified:
- `client/src/features/presence/PresenceProvider.tsx` — `readyProjects`,
  cache invalidation on disconnect, `ready` on `ProjectPresence`.
- `client/src/features/projects/Overview.tsx` — gate on `presence.ready`.
- `client/src/features/projects/Members.tsx` — gate on `presence.ready`;
  simplified unknown-presence wording.

No database migration, no `client/src/lib/realtimeClient.ts` change, no
other files touched.

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

- **Checkpoint 4 addendum (Presence Freshness / Server Lifecycle Fix, this
  session):** see the addendum section above for full detail. Summary —
  server half: `gofmt -l .`, `go build ./...`, `go test ./...`,
  `go test -race ./...`, `go vet ./...` all clean; new tests
  `app_test.TestRunSessionSweepExitsOnContextCancellation` and
  `realtime_test.TestWaitBlocksUntilTransitionDispatcherExits` both pass.
  Client half: `npm run build` (`tsc && vite build`) clean; no automated
  client tests exist to run (see Known Issues). `git diff --check` clean
  for both halves. No commit/push.
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

**Checkpoint 4 addendum (Presence Freshness / Server Lifecycle Fix, this
session):**

- **`App` owns a private child context (`appCtx`/`cancel`) rather than
  relying on the `ctx` passed into `New`.** That `ctx` is the process's
  signal-derived context (`signal.NotifyContext` in `main.go`), which is
  never cancelled on a non-signal shutdown path, and `main.go`'s own defer
  ordering (`kmjgApp.Close()` runs before `stop()`) can't be relied on
  either. Giving `App` its own cancel func makes `Close()` correct
  independent of both.
- **`Close()` waits for background goroutines (`wg.Wait()` /
  `Realtime.Wait()`) instead of just cancelling and returning.** Cancelling
  a context only *asks* a goroutine to stop; only joining it confirms it
  has. Without the join, `Pool.Close()` could still race an in-flight query
  from `runSessionSweep` or the Hub's transition dispatcher.
- **Per-Project `ready` flag on the client, not a "last known good" cache
  or a synthetic third Online/Offline/Unknown enum value.** The
  requirement was explicit: never overload the Online/Offline booleans to
  represent "unknown." A separate boolean keyed by whether *this specific
  Project's* `presence.snapshot` has landed on the *current* connection is
  the minimal signal that answers "can `isOnline` be trusted right now,"
  without touching the wire protocol or the snapshot/update event shapes.
- **Clearing `projects`/`readyProjects` is keyed off the connection-status
  transition, not off every render or every reconnect *attempt*.** The
  requirement that normal navigation on the same live connection must not
  discard a valid snapshot ruled out clearing on any broader trigger;
  `RealtimeClient` only ever reports `"reconnecting"`/`"closed"` when the
  underlying socket actually drops, so gating the clear on that transition
  is both sufficient and exactly scoped to real connectivity loss.

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

- **No server-side WebSocket connection-attempt rate limiting** (see
  Implementation Decisions above) — accepted gap for current target scale.
- **WebSocket server-initiated closes don't send a graceful close frame**
  (see Implementation Decisions above) — cosmetic close-code gap only.
- Docker, PostgreSQL 16 and Cargo/Rust are installed locally. The agent
  sandbox cannot access Docker's socket because it strips the `docker`
  supplementary group. The host-side Docker Compose smoke test is now verified
  successfully; this is an agent-sandbox limitation only.

## Next Steps (superseded — see "Next Steps" under the top checkpoint)

This was the 2026-09-25 checkpoint's next-steps list, kept here only as a
historical record. It is narrower than and superseded by the "Next Steps"
section under **Live Verification, Docs Audit, and Desktop Build
(2026-09-26)** at the top of this file — read that one, not this one, for
what to do next.

1. Put the published Server behind trusted HTTPS before using it over a
   network; configure allowed origins for the actual deployment.
2. Commit and push this release-readiness checkpoint.
