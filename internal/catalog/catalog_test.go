package catalog

import "testing"

func TestV1CatalogIsClassified(t *testing.T) {
	want := []string{
		"bash", "sh", "cd", "pwd", "echo", "printf", "env", "export", "unset", "history", "alias", "type", "which", "whereis", "man", "help", "clear", "exit", "true", "false", "test", "xargs", "tee", "cat", "tac", "head", "tail", "less", "more", "grep", "egrep", "fgrep", "sed", "awk", "cut", "sort", "uniq", "tr", "wc", "column", "paste", "diff", "cmp", "strings", "file", "base64", "sha256sum", "md5sum", "date", "watch", "time",
		"ls", "stat", "find", "locate", "touch", "mkdir", "rmdir", "cp", "mv", "rm", "ln", "readlink", "realpath", "chmod", "chown", "chgrp", "umask", "getfacl", "setfacl", "getfattr", "setfattr", "restorecon", "chcon", "tar", "gzip", "gunzip", "vi", "vim", "nano",
		"whoami", "id", "who", "w", "last", "lastlog", "groups", "getent", "su", "sudo", "passwd", "useradd", "usermod", "userdel", "groupadd", "groupmod", "groupdel", "chage", "faillock", "visudo",
		"ps", "pstree", "top", "free", "uptime", "vmstat", "iostat", "mpstat", "pidstat", "pmap", "lsof", "kill", "killall", "pkill", "pgrep", "nice", "renice", "nohup", "jobs", "fg", "bg", "dmesg", "sysctl", "ulimit",
		"systemctl", "journalctl", "loginctl", "hostnamectl", "timedatectl", "localectl", "systemd-analyze", "logger", "logrotate", "coredumpctl",
		"df", "du", "lsblk", "blkid", "findmnt", "mount", "umount", "fdisk", "parted", "mkfs", "fsck", "tune2fs", "xfs_info", "xfs_repair", "pvs", "vgs", "lvs", "pvcreate", "vgcreate", "lvcreate", "lvextend", "lvremove", "swapon", "swapoff",
		"ip", "ss", "ping", "tracepath", "traceroute", "dig", "host", "nslookup", "nmcli", "nmtui", "hostname", "curl", "wget", "nc", "ncat", "telnet", "ssh", "scp", "sftp", "tcpdump", "ethtool", "arp", "route", "netstat", "firewall-cmd", "nft", "iptables",
		"rpm", "dnf", "yum", "repoquery", "subscription-manager", "alternatives",
		"getenforce", "setenforce", "sestatus", "semanage", "ls", "ps", "id", "restorecon", "chcon", "getsebool", "setsebool", "ausearch", "aureport", "auditctl", "sealert", "openssl", "gpg", "ssh-keygen",
		"nmap", "strings", "rpm", "apachectl", "httpd", "nginx", "mysql", "psql", "redis-cli", "crontab", "at", "lab",
	}
	for _, name := range want {
		entry, ok := Lookup(name)
		if !ok {
			t.Fatalf("v1 command %q is not classified", name)
		}
		if entry.Pack == "" || entry.Level == "" || entry.Summary == "" {
			t.Fatalf("v1 command %q has incomplete classification: %+v", name, entry)
		}
	}
}

func TestEntriesAreSorted(t *testing.T) {
	entries := Entries()
	for i := 1; i < len(entries); i++ {
		if entries[i-1].Name > entries[i].Name {
			t.Fatalf("entries are not sorted at %q and %q", entries[i-1].Name, entries[i].Name)
		}
	}
}
