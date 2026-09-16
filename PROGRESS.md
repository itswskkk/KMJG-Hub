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

First vertical slice, per direct instruction from the project owner:

```
Desktop Client → Connect Server → Login → Load Projects → Select Project → Overview
```

This checkpoint covers only the **Connect Server** step.

## Completed

- **Connect Server screen** (`client/src/features/connect-server/`)
  - Matches `docs/UX.md` "Server Connection Experience → First Launch":
    heading, description, single server-address input, primary
    "Connect to Server" action.
  - Client-side only: normalizes bare hostnames (`hub.example.com`) to
    `https://hub.example.com`, validates the result as a real hostname/URL,
    shows an inline error for invalid input, clears the error as the user
    edits the field.
  - `client/src/App.tsx` holds a minimal `serverAddress` state and renders
    `ConnectServer` until an address is submitted; on success it shows a
    plain placeholder screen ("Server Selected" / "Server address: <url>"
    / "Login is not implemented yet.") as the seam for the next slice.
    Wording deliberately avoids saying "Connected" — no network call has
    been made yet, so nothing has actually confirmed the server is
    reachable. Only the address has been validated and selected.
- Removed leftover unused Tauri/React template styles from
  `client/src/App.css` (logo hover effects, `#greet-input`, unused link
  styles) that no longer applied once the template markup was replaced.

## In Progress

- Nothing in-flight. This checkpoint is complete and awaiting review.

## Files Changed

- `client/src/App.tsx` — renders `ConnectServer`, holds post-connect
  placeholder state.
- `client/src/App.css` — pruned unused template styles.
- `client/src/features/connect-server/ConnectServer.tsx` — new screen.
- `client/src/features/connect-server/ConnectServer.css` — new styles.
- `client/src/lib/serverAddress.ts` — new address normalize/validate utility.
- `PROGRESS.md` — this file (new).

## Tests / Build Checks (most recent)

- `npm run build` (runs `tsc && vite build`) in `client/` — **passes**,
  no type errors, no warnings.
- Manually exercised in a live Vite dev server via browser automation:
  - Invalid input (`"not a url"`) → inline error shown, does not proceed.
    (Caught and fixed a real bug here — see Known Issues/decisions below.)
  - Editing the field after an error clears the error.
  - Valid input (`hub.example.com`) → normalizes to
    `https://hub.example.com/` and transitions to the placeholder
    "Server Selected" state.
- No automated test suite exists yet for the client (no test runner
  configured in `package.json`).

## Implementation Decisions Made

These are simple-for-v1 choices made where the docs didn't lock in a detail.
Revisit if a future slice's requirements make them wrong.

- **No network call on "Connect".** Architecture doesn't define a specific
  health-check/version endpoint, and building one now would mean writing
  Go server logic, which is out of scope for this checkpoint. The Connect
  Server step only validates and normalizes the address client-side; actual
  server reachability will naturally surface once Login makes its first
  real API call in the next slice.
- **No "Saved Servers" list/persistence yet.** `docs/UX.md` describes a
  returning-user "Your Servers" screen backed by the Client's SQLite cache
  (per `docs/ARCHITECTURE.md`). That requires the Tauri native layer +
  local storage, which is a separate, larger unit of work. Deferred
  intentionally rather than half-built.
- **No router library added.** Only one real screen exists so far
  (`ConnectServer`); `App.tsx` does plain conditional rendering
  (`connectedServer` state). A router should be introduced when Login,
  Projects, and Overview screens land and real navigation is needed —
  don't want to guess at route structure prematurely.
- **Hostname validation is intentionally strict.** `new URL()` alone is too
  lenient (it silently percent-encodes spaces, e.g. `"not a url"` became a
  "valid" URL). Added a hostname regex
  (`^([a-z0-9]([a-z0-9-]*[a-z0-9])?)(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$`)
  plus an explicit whitespace rejection in
  `client/src/lib/serverAddress.ts`. This only covers standard hostnames;
  it does not yet special-case IPv6 literals — not needed for this
  checkpoint, worth a look if IPv6 server addresses matter later.

## Known Issues / Blockers

- None blocking. Nothing currently broken.

## Next Steps

1. Build the **Login** screen per `docs/UX.md` "Authentication Experience"
   (username/email + password, "Continue with GitHub", "Create Account"),
   wired to whatever the real Connect Server → Login handoff should be.
2. This is the natural point to decide the actual Client↔Server HTTP
   contract for auth (`docs/ARCHITECTURE.md` "Authentication Architecture"),
   since Login is the first screen that truly needs the Go server to exist.
3. Revisit "Saved Servers" (list + persistence) once local storage
   (SQLite via Tauri) is introduced — likely bundled with session/token
   storage work, since both need the native layer.
