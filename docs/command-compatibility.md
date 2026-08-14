# Command compatibility

This catalog uses four levels:

- **A — scenario-grade:** behavior and state changes needed by supported scenarios are tested.
- **B — common inspection:** common read-only options are implemented; unsupported options fail realistically.
- **C — recognized:** the command is known and explains the missing behavior.
- **Unavailable:** not exposed unless a scenario enables its package.

The initial vertical slice is intentionally small and safe. The catalog grows by adding injected command implementations and tests; it never falls back to the organizer host shell.

| Pack | Commands | Level |
| --- | --- | --- |
| shell | `cd`, `pwd`, `echo`, `printf`, `date`, `uname`, `which`, `type`, `export`, `unset`, `env`, `true`, `false`, `test`, `wc`, `sort`, `uniq`, `cut`, `history` | A/B |
| files | `ls`, `cat`, `head`, `tail`, `grep`, `stat`, `mkdir`, `touch`, `rm`, `chmod`, `chown`, `cp`, `mv`, `rmdir`, `find`, `tree`, `realpath`, `basename`, `dirname` | A/B |
| identity | `whoami`, `id`, `groups`, `sudo`, `usermod`, `useradd` | A/B |
| systemd | `systemctl`, `journalctl`, `logger` | A |
| networking | `ip`, `ss`, `ping`, `dig`, `curl`, `firewall-cmd`, `hostname`, `host`, `nslookup` | A/B |
| packages/processes/storage | `rpm`, `dnf`, `ps`, `df`, `du`, `free`, `uptime` | A/B |
| selinux | `getenforce`, `setenforce`, `sestatus`, `restorecon` | A |
| lab | `lab briefing`, `lab objectives`, `lab status`, `lab hint`, `lab note`, `lab answer`, `lab check`, `lab evidence`, `lab reset`, `lab submit` | A |

The complete v1 catalog is exposed by `redlab catalog commands`. Every command in the product-plan surface is classified in code and checked by `internal/catalog/catalog_test.go`; Level C entries are recognized safely and return an explicit unsupported-depth result. `rpm -Va` is represented as an option of the injected `rpm` command rather than as a separate command name. The catalog is the compatibility contract, and unsupported behavior never falls back to the organizer host.
