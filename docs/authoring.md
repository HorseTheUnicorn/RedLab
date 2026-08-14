# Scenario authoring

Run `redlab scenario init <directory>` for a working package template. Edit `scenario.yaml` and files under `files/`, then validate and test:

```text
redlab scenario validate ./scenarios/broken-httpd
redlab scenario test ./scenarios/broken-httpd
redlab scenario pack ./scenarios/broken-httpd
redlab scenario inspect ./scenarios/broken-httpd.rlab
```

Scenario YAML is strict. Unknown fields fail validation, and only registered `type` values are meaningful to the rule engine. A package is immutable once an event starts; teams receive virtual state derived from it.

Package paths are relative POSIX paths. Absolute paths, `..` traversal, symlinks, duplicate archive entries, oversized files, and excessive archive expansion are rejected. `files/` content is imported into virtual paths; it is never interpreted as a host path during participant execution.

The built-in `broken-httpd` package demonstrates multiple repairs: participants can edit the virtual configuration through supported commands, start the service, and allow only the required firewall service. Objectives inspect final state, so valid command sequences are not tied to a single transcript.
