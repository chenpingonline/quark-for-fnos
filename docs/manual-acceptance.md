# End-to-end acceptance test checklist (test NAS + own authorized Quark account)

Record model, CPU architecture, fnOS version, Quark App version, subscription tier, FPK SHA256, exact failure logs **without cookies/tokens/links**. Never provide third-party login credentials.

## Installation / integration

- [ ] On x86_64 fnOS, official fnpack package installs and launcher icon opens.
- [ ] On ARM64 fnOS, official fnpack package installs and launcher icon opens.
- [ ] Anonymous and non-admin requests to gateway REST endpoints return 403.
- [ ] Admin requests over Unix socket work without opening a TCP management port.
- [ ] Application starts, stops and reports status from fnOS App Center.
- [ ] Upgrade retains account state, settings, tasks and final download files.
- [ ] Data-share path exists, is writable by package user and cannot escape authorized directories.

## QR / personal cloud

- [ ] Open QR screen -> Quark App scans -> confirm -> account nickname displayed.
- [ ] QR expiration and user cancellation display a recoverable error.
- [ ] Relogin after expiration without reinstalling the FPK.
- [ ] Browse root and folders (at least 100 files, pagination), search and download search result.
- [ ] Bulk-select across pages and recursively download a nested folder, preserving hierarchy.
- [ ] Download the same remote file twice; distinct filenames, no overwrite.

## Shares

- [ ] Resolve share URL without extraction code and with correct extraction code.
- [ ] Wrong extraction code or expired share displays a clear failure.
- [ ] Browse nested share folders and selectively download files.
- [ ] Verify remote temporary saved file ID is retained until local download completes.
- [ ] With cleanup ON, only the application-created saved ID is deleted after success.
- [ ] With cleanup OFF, retained cloud copies remain visible.
- [ ] Quota shortage produces an explicit failure; no user files are deleted.

## Download engine / fault injection

- [ ] Single and multi-part HTTP 206 streams produce byte-for-byte identical SHA256 to origin.
- [ ] Server sending 200 instead of 206 triggers safe sequential fallback.
- [ ] 404/403/429, 5xx, mid-request TCP reset and signed URL expiry are surfaced/retried appropriately.
- [ ] Suspend then resume while chunks are in-flight (including task waiting for semaphore).
- [ ] Kill process mid-chunk; restart preserves completed verified ranges, discards partial chunk.
- [ ] Flip a byte in an already committed .part range; restart detects and re-downloads that range.
- [ ] Simulate crash after final file publish before status persist; restart marks task complete.
- [ ] Simulate disk-full, permission revoked, and disconnected disk: no false COMPLETED.
- [ ] Verify speed, percentage, concurrency and global rate cap on large downloads.

## Release gate

- [ ] `go test -race ./...` passes
- [ ] Official fnpack build runs successfully
- [ ] Both architectures have installation and upgrade evidence
- [ ] No plaintext credentials or share tokens in state file or logs
- [ ] All open blockers are documented in release notes

## v0.1.2 appearance / theme acceptance

1. Set fnOS desktop to dark, launch QuarkDrive, check **我的网盘 / 分享下载 / 下载任务 / 应用设置** all use dark surfaces including the login QR modal (QR canvas remains white).
2. Keep QuarkDrive open and change fnOS dark → light → dark; while **跟随飞牛系统** is selected, verify immediate appearance synchronization without reopening.
3. Choose **浅色模式** in QuarkDrive settings while fnOS is dark; QuarkDrive should remain light during host changes and after reloading the iframe.
4. Switch to **深色模式**, reload and verify persistence. Restore **跟随飞牛系统**.
5. Open QuarkDrive outside the fnOS iframe (in an allowed standalone/test environment), verify browser OS theme fallback and no uncaught cross-origin exceptions.
6. Check modal text, disabled buttons, table header, form placeholders, focus ring, checkbox and navigation contrast in both modes.

Live fnOS theme behavior depends on host version and browser embedding behavior and requires on-device validation.
