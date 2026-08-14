# RedLab

RedLab is a deterministic RHEL 8 behavioral emulator for troubleshooting and defensive-security hackathons. It runs participant commands against virtual state only; it never forwards commands to the host shell or maps virtual paths onto the host filesystem.

The repository is intentionally organized by security boundary:

- `internal/system` owns virtual RHEL state and has no host process or network execution path.
- `internal/shell` parses a deliberately bounded shell subset.
- `internal/command` contains injected, testable command packs.
- `internal/scenario` loads strict declarative YAML and validates package boundaries.
- `internal/evidence` and `internal/report` produce tamper-evident judge artifacts.
- `internal/server` is the optional organizer backend; `redlab play` is local practice.

## Quick start

The participant workflow is local-first and does not require a server:

```text
go run ./cmd/redlab play ./scenario-packs/core/broken-httpd --export ./submission.rlab.zip
```

Inside the terminal, run `lab submit` before `exit`. The signed `.rlab.zip` can be sent to the organizer out-of-band. For a coordinated remote event, an organizer may optionally run `redlab serve --event ./event/event.yaml`; participants then use `redlab join <server-url>`.

The command catalog is deliberately versioned. Unsupported behavior is reported as unsupported rather than being executed on the organizer's operating system.

## Validation

```text
go test ./...
go vet ./...
go test -race ./...
```

See `docs/architecture.md`, `docs/threat-model.md`, `docs/command-compatibility.md`, and `docs/authoring.md` for the design and current compatibility boundary.
