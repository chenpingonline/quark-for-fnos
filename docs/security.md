# Security model and known risks

- Production uses a Unix domain socket under the fnOS target directory. Requests must enter through fnOS gateway, which supplies `X-Trim-Isadmin` and `X-Trim-Userid`. If any untrusted process can directly connect to the Unix socket, it can forge these headers; check permissions on real fnOS.
- `QUARKDRIVE_DEV=1` disables the gateway check and listens on localhost. Do not expose dev mode.
- Mutation APIs require `X-QuarkDrive-Request: 1`; same-origin `Origin` is validated when supplied. This is a defense-in-depth measure, not a replacement for gateway authentication and deployment controls.
- Local auth credentials and share access tokens are encrypted with AES-256-GCM using a key file next to persisted state (permissions 0600). System administrator access to the NAS can reveal both key and data.
- All returned task JSON strips share tokens. The QR session QR URL is delivered only to the authenticated session UI. Do not put credentials or signed download URLs in logs.
- Download URL validation allows HTTPS/443 and public IP destinations. Redirects are restricted and the transport revalidates DNS at socket connect to reduce SSRF/DNS-rebinding risk.
- Server validates remote filenames, rejects absolute/traversal/backslash paths and existing symlinks within resolved destination. It cannot guarantee defense against malicious on-device users concurrently swapping directories; OS-based `openat2` confinement would be an appropriate hardening step before production.
- Tasks are bound to the destination at creation. Ensure each destination remains an authorized fnOS path if permissions change; full dynamic ACL revocation monitoring is not implemented.
- Downloads are staged in `.quarkdrive-part`. Completed ranges are SHA256 checked at restart; full file is locally hashed. Without an independent cloud-provided checksum, local SHA does not prove equivalence to the remote original.
- Completed file is created using no-clobber hard link (requires a filesystem supporting hard links). Crash after publish can be reconciled using the saved SHA256.
- Deleting a share saved file uses the recorded ID created by the app; remote task directories can remain if cleanup fails.
- Account or share sessions depend on undocumented third-party interfaces which can change. Do not attempt to evade service restrictions.
