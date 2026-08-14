# Implementation status

This repository currently provides a tested vertical slice through the core phases:

- Phase 0: Go module, CLI, CI, ADRs, architecture, and threat model.
- Phase 1: strict event/scenario YAML loading, schemas, semantic validation, path-safe packages, init/validate/pack/inspect.
- Phase 2: deterministic virtual filesystem, identities, services, journal, network, firewall, packages, and SELinux mode.
- Phase 3: bounded shell parser and injected terminal commands; no host-shell fallback.
- Phase 4: registered declarative conditions/effects, objectives, guardrails, hints, and scoring.
- Phase 5: hash-chained evidence, redacted reports, signed `.rlab.zip` bundles, and verification.
- Phase 6: SQLite migrations, signed short-lived access tokens, rotating persisted refresh tokens, authenticated REST/WebSocket session API, loopback serve, LAN TLS generation, join, submission persistence, event-log session recovery, schedule-aware lifecycle, restart limits, organizer review/judging APIs, preflight, and a built-in dashboard.
- Phase 7: all ten first-party scenario packs have declarative reference solutions, including Atlassian Jira/Confluence, GitLab, Cascade, and Drupal incidents; the catalog includes additional virtual `date`, text-processing, file-management, identity, DNS, package, process, and storage inspection commands without host execution.

Each first-party pack is exercised by `redlab scenario test`: the initial fault must score below the reference repair, alternate solutions must reach the same deterministic score, guardrails must penalize unsafe workarounds, and deterministic replay must produce the same virtual state. Local/offline signed bundle export and optional coordinated remote hosting are both supported. The repository includes a 40-session interactive load gate plus archive, parser, protocol, authentication, role-isolation, and host-boundary tests.

Race testing is wired into CI, but the current Windows environment has CGO disabled and no C compiler, so `go test -race ./...` cannot execute locally here.
