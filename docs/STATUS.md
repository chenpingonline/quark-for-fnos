# v0.1.1 Engineering Status

Date: 2026-10-14.

## Verified in this development environment

- Linux amd64 and arm64 Go cross-compilation with `CGO_ENABLED=0`.
- Local Go unit tests including 206/Content-Range, non-Range fallback, restart checkpoint loss, SHA/secret persistence, wrong 200 response, input path safety, admin gate and paused task cancellation.
- Go race detector passes for all test scenarios (`go test -race`).
- Browser JavaScript parse check passes (`node --check`).
- Local HTTP static assets and REST settings health responses (not a browser end-to-end test).
- Candidate FPK archives assemble and pass the checksum/container structure verifier.

## Implemented, requires external validation

- Quark web QR authentication and cookie lifecycle with live Quark App.
- Account lookup and actual listing/search response compatibility.
- Share link token, file listing, save-as, returned ID, cloud cleanup and refresh.
- Real Quark signed download links, expiration behavior, VIP/account quota behavior.
- fnOS identity headers, gateway socket routing, data-share grant, lifecycle callbacks.
- fnOS GUI install/update/uninstall; real AMD64/ARM64 hardware.

## Outstanding production requirements

1. Official `fnpack` build and successful clean-device install/upgrade testing. The `_UNVERIFIED.fpk` artifacts are not official releases.
2. Quark integration tests with an authorized account and share links. No guarantee that private API responses have not changed.
3. Migrate JSON state to SQLite/WAL with schema migration and crash testing at scale.
4. Port self-contained UI to Vue 3 + TypeScript if the original architecture is mandatory. Functional v0.1 UI currently uses plain JavaScript and a polling mechanism instead of WebSocket.
5. Validate permissions, sandbox boundaries, symlink race-hardening, multitenant access, and large-file performance on a real fnOS installation.
6. Expand share-token refresh, reauthorization, cloud temporary-directory reconciliation and quota-aware batched saves.
7. Real-world benchmarking and prolonged fault injection for 100+ tasks, power loss, network outages and partial disk full.

Do not describe this release as a production-certified fnOS app or as guaranteed compatible with current Quark servers.

## v0.1.1 patch

- Fixed the fnOS gateway Origin/Host mismatch for browser requests with `Sec-Fetch-Site: same-origin`; requests from cross-site contexts still fail, and the existing administrator and app header checks remain mandatory.
- QR dialog displays the API failure inline with retry instructions.
- Added `origin_test.go` with proxy, cross-site, missing marker and non-admin cases.
- No official fnpack build or actual Quark/fnOS account validation was performed as part of this patch.
- Full requirement audit: `docs/REQUIREMENTS_AUDIT.md`.

### v0.1.2: UI theme follow-up (2026-10-14)

- Added fnOS theme preference bridge and immediate light/dark rendering.
- Automatically checks same-origin fnOS iframe storage / desktop attributes, falling back to OS appearance when unavailable.
- Watches storage and host DOM updates; supports manual light/dark override via **应用设置**.
- Added dark styles for login modal, all views, forms, tables and task status; QR canvas intentionally remains black on white.
- Added `tests/theme.test.cjs` (7 passing isolated tests). Hardware / real fnOS live theme switching remains **unverified**.
