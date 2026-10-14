# HTTP API v0.1 (development contract)

Production base path: `/app/quarkdrive/api/v1`. Local dev mode base path: `/api/v1`.

A real fnOS administrator must use the platform gateway. For POST, PUT or DELETE, the browser sends `X-QuarkDrive-Request: 1`, Content-Type JSON, same-origin credentials.

| Method | Path | Description |
|---|---|---|
| GET | `health` | Version/status |
| GET | `auth/account` | Current Quark account (validates remote session) |
| DELETE | `auth/account` | Logout/delete encrypted credentials |
| POST | `auth/qr-sessions` | New QR login session |
| GET | `auth/qr-sessions/{id}` | Poll QR login state |
| GET | `files?parent=0&page=1` | Personal directory page |
| GET | `files/search?q=...&page=1` | Search personal drive |
| POST | `shares/previews` | JSON `{ "url": "https://pan.quark.cn/s/…", "code": "" }` |
| GET | `shares/previews/{id}/files?parent=0&page=1` | Share tree page |
| POST | `tasks` | Create one or more file/folder download tasks |
| GET | `tasks` | All task status + speed/bytes (tokens redacted) |
| GET | `tasks/{id}` | Task detail |
| POST | `tasks/{id}/pause` | Pause and wait for cancellation |
| POST | `tasks/{id}/resume` | Queue a paused task |
| POST | `tasks/{id}/retry` | Queue a failed task |
| GET/PUT | `settings` | Download configuration (full Settings JSON) |
| GET | `storage/locations` | Allowed fnOS data-share paths |

Creating tasks:

```json
{
  "source_kind": "personal",
  "parent": "0",
  "preview_id": "",
  "search": "",
  "selected": [{"id":"fid","name":"demo.bin","folder":false,"size":42}],
  "subdir": ""
}
```

For a share task set `source_kind:"share"` and provide a fresh `preview_id` obtained from the preview API. `selected` fields are revalidated against Quark before creating tasks. Folder recursion is server-side. In the current implementation, download location comes from persisted settings; arbitrary location paths cannot be injected through the create task request.

Errors: `{ "error": "..." }` with HTTP status. Task states: `QUEUED`, `RESOLVING`, `DOWNLOADING`, `VERIFYING`, `RETRY_WAIT`, `FAILED`, `PAUSED`, `COMPLETED`. Task progress currently uses a REST polling UI rather than WebSocket.
