package catalog

import "sort"

type Level string

const (
	LevelA Level = "A"
	LevelB Level = "B"
	LevelC Level = "C"
)

type Entry struct {
	Name        string   `json:"name" yaml:"name"`
	Pack        string   `json:"pack" yaml:"pack"`
	Level       Level    `json:"level" yaml:"level"`
	Summary     string   `json:"summary" yaml:"summary"`
	Options     []string `json:"options,omitempty" yaml:"options,omitempty"`
	Implemented bool     `json:"implemented" yaml:"implemented"`
}

var entries = []Entry{
	{Name: "pwd", Pack: "shell", Level: LevelA, Summary: "Print the virtual working directory", Implemented: true},
	{Name: "cd", Pack: "shell", Level: LevelA, Summary: "Change the virtual working directory", Implemented: true},
	{Name: "bash", Pack: "shell", Level: LevelA, Summary: "Enter the bounded RedLab shell", Implemented: true},
	{Name: "sh", Pack: "shell", Level: LevelA, Summary: "Enter the bounded RedLab shell", Implemented: true},
	{Name: "alias", Pack: "shell", Level: LevelB, Summary: "Inspect the bounded shell alias table", Implemented: true},
	{Name: "help", Pack: "shell", Level: LevelA, Summary: "Show RedLab command help", Implemented: true},
	{Name: "man", Pack: "shell", Level: LevelB, Summary: "Show a compatibility entry", Implemented: true},
	{Name: "clear", Pack: "shell", Level: LevelA, Summary: "Clear the virtual terminal", Implemented: true},
	{Name: "exit", Pack: "shell", Level: LevelA, Summary: "Leave the bounded shell", Implemented: true},
	{Name: "echo", Pack: "shell", Level: LevelA, Summary: "Write arguments", Implemented: true},
	{Name: "printf", Pack: "shell", Level: LevelA, Summary: "Format text", Implemented: true},
	{Name: "date", Pack: "shell", Level: LevelA, Summary: "Print the deterministic virtual clock", Implemented: true},
	{Name: "history", Pack: "shell", Level: LevelA, Summary: "Print command history", Implemented: true},
	{Name: "which", Pack: "shell", Level: LevelA, Summary: "Resolve a command in the compatibility catalog", Implemented: true},
	{Name: "type", Pack: "shell", Level: LevelA, Summary: "Describe a command in the compatibility catalog", Implemented: true},
	{Name: "whereis", Pack: "shell", Level: LevelB, Summary: "Locate a command in the virtual image", Implemented: true},
	{Name: "wc", Pack: "shell", Level: LevelB, Summary: "Count virtual text lines, words, and bytes", Implemented: true},
	{Name: "sort", Pack: "shell", Level: LevelB, Summary: "Sort virtual text lines", Implemented: true},
	{Name: "uniq", Pack: "shell", Level: LevelB, Summary: "Collapse adjacent duplicate lines", Implemented: true},
	{Name: "cut", Pack: "shell", Level: LevelB, Summary: "Select delimited virtual text fields", Implemented: true},
	{Name: "env", Pack: "shell", Level: LevelA, Summary: "Print the injected environment", Implemented: true},
	{Name: "export", Pack: "shell", Level: LevelA, Summary: "Set an injected environment variable", Implemented: true},
	{Name: "unset", Pack: "shell", Level: LevelA, Summary: "Remove an injected environment variable", Implemented: true},
	{Name: "true", Pack: "shell", Level: LevelA, Summary: "Return success", Implemented: true},
	{Name: "false", Pack: "shell", Level: LevelA, Summary: "Return failure", Implemented: true},
	{Name: "test", Pack: "shell", Level: LevelA, Summary: "Evaluate basic virtual filesystem predicates", Implemented: true},
	{Name: "ls", Pack: "files", Level: LevelA, Summary: "List virtual filesystem entries", Implemented: true},
	{Name: "cat", Pack: "files", Level: LevelA, Summary: "Read virtual files", Implemented: true},
	{Name: "head", Pack: "files", Level: LevelB, Summary: "Read the first lines of a virtual file", Implemented: true},
	{Name: "tail", Pack: "files", Level: LevelB, Summary: "Read the last lines of a virtual file", Implemented: true},
	{Name: "grep", Pack: "files", Level: LevelA, Summary: "Search virtual file content or stdin", Implemented: true},
	{Name: "tee", Pack: "files", Level: LevelA, Summary: "Write stdin to virtual files and stdout", Implemented: true},
	{Name: "stat", Pack: "files", Level: LevelB, Summary: "Inspect virtual metadata", Implemented: true},
	{Name: "mkdir", Pack: "files", Level: LevelA, Summary: "Create a virtual directory", Implemented: true},
	{Name: "touch", Pack: "files", Level: LevelA, Summary: "Create a virtual file", Implemented: true},
	{Name: "rm", Pack: "files", Level: LevelA, Summary: "Remove virtual entries", Implemented: true},
	{Name: "chmod", Pack: "files", Level: LevelA, Summary: "Change virtual mode bits", Implemented: true},
	{Name: "chown", Pack: "files", Level: LevelA, Summary: "Change virtual ownership", Implemented: true},
	{Name: "cp", Pack: "files", Level: LevelA, Summary: "Copy a virtual file or directory tree", Options: []string{"-r", "-R", "--recursive"}, Implemented: true},
	{Name: "mv", Pack: "files", Level: LevelA, Summary: "Move a virtual file or directory tree", Implemented: true},
	{Name: "rmdir", Pack: "files", Level: LevelB, Summary: "Remove an empty virtual directory", Implemented: true},
	{Name: "find", Pack: "files", Level: LevelB, Summary: "Find entries in the virtual filesystem", Implemented: true},
	{Name: "basename", Pack: "files", Level: LevelA, Summary: "Strip directory components from a virtual path", Implemented: true},
	{Name: "dirname", Pack: "files", Level: LevelA, Summary: "Strip the final component from a virtual path", Implemented: true},
	{Name: "realpath", Pack: "files", Level: LevelB, Summary: "Resolve a canonical virtual path", Implemented: true},
	{Name: "tree", Pack: "files", Level: LevelB, Summary: "Display a virtual directory tree", Implemented: true},
	{Name: "whoami", Pack: "identity", Level: LevelA, Summary: "Print the virtual effective user", Implemented: true},
	{Name: "id", Pack: "identity", Level: LevelA, Summary: "Print virtual identity information", Implemented: true},
	{Name: "groups", Pack: "identity", Level: LevelA, Summary: "Print virtual group membership", Implemented: true},
	{Name: "sudo", Pack: "identity", Level: LevelA, Summary: "Run a command under simulated sudo policy", Implemented: true},
	{Name: "usermod", Pack: "identity", Level: LevelA, Summary: "Modify virtual user group membership", Implemented: true},
	{Name: "useradd", Pack: "identity", Level: LevelB, Summary: "Create a virtual user", Implemented: true},
	{Name: "systemctl", Pack: "systemd", Level: LevelA, Summary: "Inspect and mutate virtual services", Implemented: true},
	{Name: "journalctl", Pack: "systemd", Level: LevelA, Summary: "Inspect the virtual journal", Implemented: true},
	{Name: "logger", Pack: "systemd", Level: LevelA, Summary: "Append to the virtual journal", Implemented: true},
	{Name: "hostname", Pack: "networking", Level: LevelA, Summary: "Print the virtual hostname", Implemented: true},
	{Name: "host", Pack: "networking", Level: LevelB, Summary: "Resolve a scenario-defined virtual host", Implemented: true},
	{Name: "nslookup", Pack: "networking", Level: LevelB, Summary: "Resolve a scenario-defined virtual host", Implemented: true},
	{Name: "ip", Pack: "networking", Level: LevelB, Summary: "Inspect virtual interfaces", Implemented: true},
	{Name: "ss", Pack: "networking", Level: LevelB, Summary: "Inspect virtual sockets", Implemented: true},
	{Name: "ping", Pack: "networking", Level: LevelB, Summary: "Probe a scenario-defined virtual host", Implemented: true},
	{Name: "dig", Pack: "networking", Level: LevelB, Summary: "Resolve scenario-defined DNS", Implemented: true},
	{Name: "curl", Pack: "networking", Level: LevelA, Summary: "Request a virtual HTTP endpoint", Implemented: true},
	{Name: "firewall-cmd", Pack: "firewalld", Level: LevelA, Summary: "Inspect and mutate virtual firewall zones", Implemented: true},
	{Name: "getenforce", Pack: "selinux", Level: LevelA, Summary: "Print virtual SELinux mode", Implemented: true},
	{Name: "setenforce", Pack: "selinux", Level: LevelA, Summary: "Change virtual SELinux enforcing mode", Implemented: true},
	{Name: "sestatus", Pack: "selinux", Level: LevelA, Summary: "Print virtual SELinux status", Implemented: true},
	{Name: "restorecon", Pack: "selinux", Level: LevelA, Summary: "Restore a virtual file context", Implemented: true},
	{Name: "rpm", Pack: "packages", Level: LevelA, Summary: "Inspect virtual installed packages", Implemented: true},
	{Name: "dnf", Pack: "packages", Level: LevelB, Summary: "Inspect virtual package state", Implemented: true},
	{Name: "ps", Pack: "processes", Level: LevelB, Summary: "List deterministic virtual processes", Implemented: true},
	{Name: "pstree", Pack: "processes", Level: LevelB, Summary: "Show the virtual process tree", Implemented: true},
	{Name: "pgrep", Pack: "processes", Level: LevelB, Summary: "Find virtual processes by command", Implemented: true},
	{Name: "pkill", Pack: "processes", Level: LevelB, Summary: "Signal virtual processes by command", Implemented: true},
	{Name: "kill", Pack: "processes", Level: LevelB, Summary: "Signal a virtual process", Implemented: true},
	{Name: "killall", Pack: "processes", Level: LevelB, Summary: "Signal virtual processes by name", Implemented: true},
	{Name: "top", Pack: "processes", Level: LevelB, Summary: "Show a deterministic virtual process summary", Implemented: true},
	{Name: "df", Pack: "storage", Level: LevelB, Summary: "Report deterministic virtual filesystem usage", Implemented: true},
	{Name: "du", Pack: "storage", Level: LevelB, Summary: "Estimate virtual file space usage", Implemented: true},
	{Name: "free", Pack: "processes", Level: LevelB, Summary: "Display deterministic virtual memory usage", Implemented: true},
	{Name: "uptime", Pack: "processes", Level: LevelB, Summary: "Display deterministic virtual uptime and load", Implemented: true},
	{Name: "uname", Pack: "shell", Level: LevelA, Summary: "Print virtual kernel information", Implemented: true},
	{Name: "lab", Pack: "lab", Level: LevelA, Summary: "Inspect and submit lab work", Implemented: true},
}

// recognized is the complete v1 catalog surface. Entries that do not yet
// have scenario-grade behavior are still classified so the CLI can explain
// the compatibility boundary instead of pretending they are host commands.
var recognized = []Entry{}

func init() {
	groups := map[string][]string{
		"shell":         {"bash", "sh", "alias", "type", "which", "whereis", "man", "help", "clear", "exit", "xargs", "tac", "less", "more", "egrep", "fgrep", "sed", "awk", "cut", "sort", "uniq", "tr", "wc", "column", "paste", "diff", "cmp", "strings", "file", "base64", "sha256sum", "md5sum", "date", "watch", "time"},
		"files":         {"find", "locate", "rmdir", "cp", "mv", "ln", "readlink", "realpath", "umask", "getfacl", "setfacl", "getfattr", "setfattr", "chgrp", "chcon", "tar", "gzip", "gunzip", "vi", "vim", "nano"},
		"identity":      {"who", "w", "last", "lastlog", "getent", "su", "passwd", "useradd", "usermod", "userdel", "groupadd", "groupmod", "groupdel", "chage", "faillock", "visudo"},
		"processes":     {"ps", "pstree", "top", "free", "uptime", "vmstat", "iostat", "mpstat", "pidstat", "pmap", "lsof", "kill", "killall", "pkill", "pgrep", "nice", "renice", "nohup", "jobs", "fg", "bg", "dmesg", "sysctl", "ulimit"},
		"systemd":       {"loginctl", "hostnamectl", "timedatectl", "localectl", "systemd-analyze", "logrotate", "coredumpctl"},
		"storage":       {"df", "du", "lsblk", "blkid", "findmnt", "mount", "umount", "fdisk", "parted", "mkfs", "fsck", "tune2fs", "xfs_info", "xfs_repair", "pvs", "vgs", "lvs", "pvcreate", "vgcreate", "lvcreate", "lvextend", "lvremove", "swapon", "swapoff"},
		"networking":    {"tracepath", "traceroute", "host", "nslookup", "nmcli", "nmtui", "hostname", "wget", "nc", "ncat", "telnet", "ssh", "scp", "sftp", "tcpdump", "ethtool", "arp", "route", "netstat", "nft", "iptables"},
		"packages":      {"rpm", "dnf", "yum", "repoquery", "subscription-manager", "alternatives"},
		"selinux":       {"ausearch", "aureport", "auditctl", "sealert", "semanage", "getsebool", "setsebool", "openssl", "gpg", "ssh-keygen"},
		"investigation": {"nmap", "rpm-vA"},
		"web":           {"apachectl", "httpd", "nginx", "mysql", "psql", "redis-cli", "crontab", "at"},
	}
	for pack, names := range groups {
		for _, name := range names {
			if _, exists := Lookup(name); exists {
				continue
			}
			recognized = append(recognized, Entry{Name: name, Pack: pack, Level: LevelC, Summary: "Recognized; this behavior is not yet emulated", Implemented: false})
		}
	}
	entries = append(entries, recognized...)
}

func Entries() []Entry {
	out := make([]Entry, len(entries))
	copy(out, entries)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func Lookup(name string) (Entry, bool) {
	for _, entry := range entries {
		if entry.Name == name {
			return entry, true
		}
	}
	return Entry{}, false
}
