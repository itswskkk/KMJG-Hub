## Resume Here

Last verified commit: `88f1cbe` (`feat: add project members view`)
Branch: `main`
Working tree after checkpoint: clean

Current implementation state:
- Connect Server: complete
- Local Auth / Sessions: complete
- Projects / Overview: complete
- Members view-only: complete

No work is currently in progress.

Next checkpoint must be selected by the project owner.
Current candidates:
- Presence / WebSocket
- Tasks
- Project Chat
- Member management / Remove Member

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
PostgreSQL-backed server. This session's checkpoint (**Members**) builds on
top of it, per direct instruction from the project owner, rather than
extending that original slice's scope.

## Completed

### Checkpoint 3: Members (this checkpoint)

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
  yet" note instead of the presence dots (`●`/`○`) the UX mockup shows —
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

## Completed (previous checkpoint)

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
  Available Yet" label, same treatment as the GitHub login stub.
- **Presence / online status (WebSocket).** `docs/ARCHITECTURE.md`
  "Presence and Work Status Architecture" explicitly requires presence be
  derived from live authenticated connections (heartbeat/timeout), not a
  stored boolean — there is no real-time transport in this codebase yet at
  all. Faking it from session-table existence would contradict the
  documented design rather than just be incomplete, so it wasn't done.
  Overview shows real member + role data and an honest "Online presence is
  not implemented yet" note instead.
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
- Members checkpoint has been reviewed, committed, and pushed.

## Files Changed

**This checkpoint (Members) — client-only, no backend/database changes:**
- `client/src/features/projects/Members.tsx`, `Members.css` — new: the
  Members list screen and the Member Detail overlay.
- `client/src/features/projects/ProjectWorkspace.tsx` — reworked sidebar
  into a real `overview`/`members` section switch; Overview and Members
  sidebar entries are now clickable buttons instead of static disabled
  `<span>`s (the other five items are unchanged, still disabled).
- `client/src/features/projects/Overview.tsx`, `Overview.css` — Members
  section simplified from a full roster to a count + "View Members" button;
  removed the now-unused per-member-row CSS.
- `PROGRESS.md` — this file.

**Previous checkpoint (Load Projects → Select Project → Overview) — unchanged
this session:**
- `server/internal/auth/service.go` — added `ErrSessionInvalid`; `CurrentUser`
  now returns it instead of `ErrInvalidCredentials`.
- `server/internal/auth/service_test.go` — updated two assertions for the
  above.
- `server/internal/httpapi/auth.go` — added the `ErrSessionInvalid` →
  `invalid_session` mapping; `handleLogout`/`handleCurrentSession` simplified
  to use `currentAuth(r)` instead of re-extracting the token themselves.
- `server/internal/httpapi/context.go` — new: `requireAuth` middleware,
  `authContext`, `currentAuth`.
- `server/internal/httpapi/server.go` — `Handlers` gained `Projects`; router
  gained the three `/api/v1/projects...` routes (wrapped in `requireAuth`)
  and now wraps `/auth/logout` + `/auth/session` in it too.
- `server/internal/httpapi/projects.go` — new: handlers + DTOs for
  create/list/get project.
  `server/internal/httpapi/projects_test.go` — new: `fakeProjectRepo` +
  create/list/get/non-member-404/unauthenticated/validation tests.
  `server/internal/httpapi/auth_test.go` — `newTestRouter` now also wires a
  `project.Service`; added `extractToken`/`decodeJSON` test helpers; the
  fake user repo grew a `usernameFor` method so the fake project repo can
  resolve member usernames the way the real repository joins against
  `users`.
- `server/internal/project/` — new package: `project.go`, `validate.go`,
  `service.go`, `service_test.go`.
- `server/internal/store/postgres/projects.go` — new: `ProjectRepository`.
  `server/internal/store/postgres/migrations/0002_projects.sql` — new.
- `server/internal/app/app.go` — wires `project.Service`.
- `client/src/lib/apiClient.ts` — added project types/calls,
  `isSessionExpired`.
- `client/src/features/server-home/{ServerHome.tsx,ServerHome.css}` — new.
- `client/src/features/projects/{CreateProject.tsx,CreateProject.css,
  ProjectWorkspace.tsx,ProjectWorkspace.css,Overview.tsx,Overview.css}` —
  new.
- `client/src/App.tsx` — extended state machine through Projects/Overview.
- `client/src/App.css` — dark-mode `textarea` fix.

**Previous checkpoint** (Connect Server → Login; unchanged this session —
see git history / the prior version of this file for full detail): all of
`server/` except the additions above, `docker-compose.yml`, `.env.example`,
root `.gitignore`, `client/src/features/auth/`, original `apiClient.ts`.

## Tests / Build Checks (most recent)

- **This checkpoint (Members):**
  - No backend changes, so no new Go tests were needed or added; re-ran the
    full existing suite as a regression check — `go build ./...`,
    `go vet ./...`, `gofmt -l .`, `go test ./...` all still pass unchanged.
  - `npm run build` (`tsc && vite build`) — passes, no type errors.
  - **Live browser walkthrough against the real running server** (the same
    `docker compose` containers from the previous checkpoint, still up —
    no rebuild needed since nothing server-side changed): logged in with
    the same real account used in the previous checkpoint's verification,
    opened "Game Center" (a real project with exactly one real member,
    since there is still no way to add a second member to a project without
    the deferred Join/Invite flow) → Overview now shows "1 Member" +
    "View Members" instead of the old inline roster → clicked it → landed
    on the new Members screen showing the real member (username + Owner
    role) → clicked the member row → Member Detail overlay opened showing
    username, role, the honest not-implemented-yet note, and all four quick
    action buttons visibly disabled → clicked the disabled "Chat" button to
    confirm it's genuinely inert (no navigation, no console error) →
    clicked outside the overlay to close it → clicked back to Overview and
    to Members again to confirm section switching doesn't re-fetch or
    flicker. No console errors at any point.
- **Previous checkpoint (Load Projects → Select Project → Overview), for
  reference — unchanged this session:**
  - `go build ./...`, `go vet ./...`, `gofmt -l .` — all clean.
  - `go test ./...` — passes. Coverage added that checkpoint:
    `internal/project` (`Service.Create` makes the creator Owner with
    exactly one member; rejects empty/whitespace-only/too-long name and
    too-long description; `List` only returns the caller's projects;
    `GetDetail` returns `ErrNotFound` for a non-member); `internal/httpapi`
    (`TestCreateAndListAndGetProject` — full create→list→get round trip
    including that the real member/username join shape comes back
    correctly; `TestGetProjectNotFoundForNonMember`;
    `TestCreateProjectRequiresAuth`; `TestCreateProjectValidatesName`).
    `internal/auth`'s `CurrentUser`-after-logout /
    `CurrentUser`-with-unknown-token tests updated to expect
    `ErrSessionInvalid`.
  - **Verified against a real PostgreSQL instance via `docker compose`, in
    this session** (this is new — the previous checkpoint could not do
    this; see below for how). `docker compose up --build` succeeded
    (Postgres healthy, migrations `0001` and `0002` applied with no
    errors); then, with real `curl` against the running container:
    - Registered two real accounts (real Argon2id hash, real UUID ids,
      really persisted in Postgres — confirmed via a fresh server restart
      picking up the same data implicitly through the flow below still
      working).
    - Created a real project (`POST /api/v1/projects`) → got back a real
      UUID, `member_count: 1`, `role: "owner"`.
    - `GET /api/v1/projects` → the project appeared in the list.
    - `GET /api/v1/projects/{id}` → full detail with the correct
      username+role, resolved through the real `JOIN` against `users`.
    - A second registered user requesting the first user's project id →
      real `404`.
    - A malformed project id (`not-a-uuid`) → real `404`, not a `500`
      (confirms the `22P02` → `ErrNotFound` mapping works against actual
      Postgres error codes, not just the assumption it would).
    - `POST /api/v1/projects` with no `Authorization` header → real `401
      unauthorized`.
    - `POST /api/v1/projects` with an empty name → real `400
      validation_error`.
    - Logout (`204`) then reusing the same token against
      `GET /api/v1/auth/session` → real `401 invalid_session` — confirms
      the session-invalid-vs-invalid-credentials fix works end-to-end
      against the real session table, not just the in-memory fakes.
  - **Docker environment:** Docker is installed and the current user has
    Docker group access. `docker ps` and `docker compose` work without `sudo`.
  - The `.env` file and running containers were already present at the
    start of this checkpoint (the project owner had independently run
    `docker compose up --build` and verified the Connect Server → Login
    checkpoint for real before handing off this checkpoint) — this session
    rebuilt the image (`docker compose up --build -d`) to pick up the new
    Projects code and reused the same Postgres volume/container, so the
    verification above is on top of that same real database, not a fresh
    throwaway one.
- **Previous checkpoint's client verification, for reference:**
  - `npm run build` (`tsc && vite build`) — passes, no type errors.
  - **Live browser walkthrough against the real running server** (no stub
    this time — Docker/Postgres access made that unnecessary): Connect to
    `http://localhost:8080` → real health check passes → Login screen;
    logged in with a real account created via `curl` earlier in the
    session → landed on Server Home showing the real project created via
    `curl` ("KMJG Hub Development", 1 Member); clicked Open → Project
    Workspace → Overview rendered the real member (username + Owner role),
    "No Repository Connected", and the honest not-implemented-yet notes for
    Tasks/Git Activity/Project Activity/online presence; navigated back to
    Server Home, used **Create Project** through the actual UI (not curl)
    with the Repository radio group correctly locked to "Set Up Later" →
    real project created → landed directly on its Overview; back to Server
    Home → both projects now listed; **Log Out** → correctly returned to
    Login. No console errors at any point. (This is also where the
    `textarea` dark-mode bug above was caught and fixed.)

## Implementation Decisions Made

**This checkpoint (Members):**

- **View-only, no member management action, by explicit project-owner
  choice** (see "Scope decision" under Completed above). "Removing a
  Project Member Experience" is real docs/UX.md scope, just not this
  checkpoint's.
- **Presence indicators (`●`/`○`) from the UX mockup were dropped entirely
  rather than rendered as a static/neutral icon.** A static dot next to
  each name would still visually read as *some* status, which is worse
  than no icon at all when there's no real status behind it — the
  checkpoint instructions were explicit about not faking data, and a
  fake-but-neutral-looking icon is still a step in that direction. A plain
  text note ("not implemented yet") can't be misread as real status.
- **Overview's Members section was edited** even though this checkpoint's
  brief was "Members," not "Overview" — justified because it was
  duplicating the exact same per-member role list the new Members screen
  now owns, and leaving that duplication in place would mean two different
  UI surfaces could drift out of sync for no reason. Scoped narrowly (only
  the Members subsection changed; Repository/Tasks/Git Activity/Project
  Activity untouched).
- **No new endpoint for the Members screen.** Reusing `GET
  /api/v1/projects/{id}` (already fetched by `ProjectWorkspace` on mount)
  keeps the checkpoint's backend footprint at zero, per "เพิ่ม backend/API/
  database changes เท่าที่จำเป็น" — there was nothing to add.

**Previous checkpoint (Load Projects → Select Project → Overview), kept
below for reference — all still accurate, none revisited this session:**

- **`project.ValidationError` duplicates `auth.ValidationError`'s shape
  instead of importing it.** Project field validation (name/description
  length) has nothing conceptually to do with credential validation;
  coupling the two packages to share a 4-line error type isn't worth the
  dependency. Same judgment call as keeping `user`/`session` independent
  packages in the previous checkpoint.
- **`project_members.role` is `TEXT CHECK (...)`, not a Postgres `ENUM`
  type.** Enums require `ALTER TYPE` gymnastics to extend later (v1 only
  has 3 roles now, but the docs describe role-related nuance — e.g. "no
  Admin can promote/demote another Admin" — that's all authorization logic,
  not a reason to expect the *role set itself* to grow, but a check
  constraint costs nothing extra now and is strictly easier to change than
  an enum if it ever does).
- **Non-member access and "project doesn't exist" both return the same 404
  `not_found`.** `docs/ARCHITECTURE.md` "Resource-Level Authorization" says
  Project data must only be accessible to authorized members; returning a
  distinguishable "403 you're not a member" would leak that a private
  project *exists* at that ID to someone who has no business knowing that.
  This was implemented by construction (the membership check is inside the
  same `JOIN` as the existence check), not as a special case to remember.
- **Malformed project IDs (not valid UUIDs) also collapse to 404** rather
  than a 400 "invalid ID format" or an uncaught 500. Treating "can't
  possibly be a real ID" the same as "not found" is simpler than adding a
  separate validation path, and avoids giving a client any signal about ID
  *format* that differs from ID *existence*.
- **Project creation always takes the "Set Up Later" repository path,
  client-side and server-side.** `CreateInput` (server) has no repository
  fields at all, and the Create Project screen (client) only lets "Set Up
  Later" be selected. This isn't a partial implementation of the other two
  radio options — they're not implemented even as a no-op — so disabling
  them in the UI is an honest reflection of server capability, not just a
  UI restriction.
- **`ServerHome`'s Log Out button calls the real logout endpoint but
  proceeds to the Login screen even if that call fails** (network error,
  already-expired token, etc.). The Client is discarding its only copy of
  the token either way; blocking the user in the app because the
  now-being-abandoned token couldn't be revoked server-side would be worse
  than the (rare, session-TTL-bounded) risk of a token outliving the
  Client's own memory of it.
- **No nav bar for Friends/DMs/Notifications/Profile on Server Home** — see
  "What was deliberately not built" above. Documented as its own decision
  because it's the one place this checkpoint chose *not* to follow the
  "render disabled, same as GitHub" pattern, and the reasoning for that
  distinction (same-UX-section stub vs. unrelated-feature stub) is worth
  preserving for whoever builds Friends/DMs next.

## Known Issues / Blockers

- **"Removing a Project Member Experience" is not implemented** (by
  explicit project-owner choice this checkpoint — see "Scope decision"
  above). Owners/Admins currently have no way to remove a member through
  the Client. This is the natural next piece if member management is
  wanted.
- **Presence, Tasks, Git activity, and Project activity are still not
  implemented** (unchanged from the previous checkpoint). Members and
  Overview are both structurally complete per `docs/UX.md` but several
  sections in each are honest placeholders, not real features.
- **Join Project (invite links/codes) is not implemented.** Only creating
  your own project gets you into one right now — which also means it was
  not possible to verify the Members screen with more than one member in
  it this checkpoint (every project in the live test database has exactly
  one member, its creator). The single-member case was verified live; the
  multi-member rendering path (list of >1 row, each independently
  clickable) was not exercised against a real multi-member project, only
  reasoned about from the code (it's a plain `.map()` over
  `detail.members`, nothing member-count-dependent in the logic) — worth a
  real check once Join Project or a second membership path exists.
- Session token is still not persisted across app restarts (unchanged,
  deferred as before).
- No automated test suite exists for the client (still unaddressed, same
  gap noted in every previous checkpoint).
- The dev Postgres volume still has the same test accounts/projects noted
  in the previous checkpoint's handoff (`korn_<timestamp>` /
  `meran_<timestamp>`, "KMJG Hub Development" / "Game Center") — untouched
  this session, no new data added.

## Next Steps

1. **Removing a Project Member Experience** (`docs/UX.md`) is the most
   direct follow-on to this checkpoint — the Members screen is already
   right there to add it to. Needs: `DELETE /api/v1/projects/{id}/members/
   {userId}` (or similar) with server-side role authorization (Owner can
   remove Admin/Member; Admin can remove Member only; nobody can remove the
   Owner; no self-removal — that's "Leaving a Project," a different UX
   section), confirmation UI, and correct 403/404 handling. The "also
   remove repository access" checkbox from the UX mockup should stay
   omitted until Git integration exists (there's never anything to remove
   access to yet).
2. Otherwise, pick one of: **Presence** (`docs/ARCHITECTURE.md` "Presence
   and Work Status Architecture" — needs the WebSocket real-time layer,
   which nothing in the codebase has yet, and unlocks the presence
   indicators both Overview and Members are currently honest about
   lacking), **Tasks** (`docs/ARCHITECTURE.md` "Task Architecture" — the
   most self-contained of the remaining pieces, doesn't need WebSocket to
   be useful since it's HTTP-API-shaped like Projects was), or **Project
   Chat** (also needs the WebSocket layer for its real-time half, though
   message history retrieval could be HTTP-only first). Confirm with the
   project owner before starting — this PROGRESS.md shouldn't be the thing
   deciding product sequencing.
3. Session persistence (SQLite via Tauri + OS credential storage) is still
   deferred — bundle it with "Saved Servers" persistence when the Tauri
   native layer work starts, as noted in every previous checkpoint. Getting
   this done would also let `GET /api/v1/auth/session` finally get used by
   the Client (it's been sitting ready since the Login checkpoint).
