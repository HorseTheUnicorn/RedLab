# Scenario authoring

Run `redlab scenario init <directory> --id <id> --title <title>` for a new working package template. This is a runtime authoring workflow: authors edit YAML and package files with the compiled binary and never need to recompile RedLab. Edit `scenario.yaml` and files under `files/`, then validate and test:

```text
redlab scenario validate ./scenarios/broken-httpd
redlab scenario test ./scenarios/broken-httpd
redlab scenario pack ./scenarios/broken-httpd
redlab scenario inspect ./scenarios/broken-httpd.rlab
```

The organizer dashboard provides the same no-recompile workflow under **Scenario Workshop**. It can create templates, import/export `.rlab` packages, validate and save `scenario.yaml`, and add or remove package files. Use the dashboard when the package should be attached to an event; use the CLI when working offline.

Scenario YAML is strict. Unknown fields fail validation, and only registered `type` values are meaningful to the rule engine. A package is immutable once an event starts; teams receive virtual state derived from it.

Package paths are relative POSIX paths. Absolute paths, `..` traversal, symlinks, duplicate archive entries, oversized files, and excessive archive expansion are rejected. `files/` content is imported into virtual paths; it is never interpreted as a host path during participant execution.

The built-in `broken-httpd` package demonstrates multiple repairs: participants can edit the virtual configuration through supported commands, start the service, and allow only the required firewall service. Objectives inspect final state, so valid command sequences are not tied to a single transcript.
