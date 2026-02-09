# Container Mode TODOs

## High Severity

- [x] **Forked containerized sessions will fail at runtime** (`internal/claude/process_manager.go:191`)
  When `Containerized=true` and `ForkFromSessionID != ""`, `BuildCommandArgs` enters the fork branch which uses `--resume <parentID> --fork-session`. Inside a container, the parent session data doesn't exist. The test `TestBuildCommandArgs_Containerized_ForkedSession` validates this broken behavior.

- [x] **Explore Options (Ctrl+O) doesn't inherit containerized flag** (`internal/app/modal_handlers_issues.go`)
  `createParallelSessions` creates child sessions but never sets `sess.Containerized = true` from the parent, silently running without container sandboxing when the user expects it. Compare with `handleForkSessionModal` which does this correctly.

## Medium Severity

- [x] **Session runs unprotected when image is missing** (`internal/app/modal_handlers_session.go`)
  Now checks image existence BEFORE creating the session. If missing, shows build instructions without creating an unprotected session.

- [x] **Broadcast silently skips containers when image missing** (`internal/app/modal_handlers_session.go`)
  Now checks image before creating sessions and shows a flash warning when container image is missing.

- [x] **Auth credentials persist in /tmp after unrecoverable crashes** (`internal/claude/process_manager.go`)
  Auth file at `/tmp/plural-auth-<session-id>` is now cleaned up in both fatal error paths (max restarts exceeded and restart failure).

## Low Severity

- [x] **Entrypoint export is fragile** (`entrypoint.sh`)
  Replaced `export "$(cat ...)"` with `set -a / source / set +a` pattern.

- [x] **Misleading "skipping MCP servers" log** (`internal/app/session_manager.go`)
  Updated log message and now skips loading MCP servers entirely for containerized sessions.

- [x] **Data race on r.containerized** (`internal/claude/claude.go`)
  Now reads `r.containerized` under lock before spawning the goroutine in `SendContent`.

- [x] **Image name not validated** (`internal/ui/modals/container_build.go`)
  Added regex validation for container image names. Invalid names fall back to default "plural-claude".

- [x] **Full ~/.claude copy on every container start** (`entrypoint.sh`)
  Now copies only needed files (settings.json, CLAUDE.md, projects/, plugins/) instead of the entire directory.
