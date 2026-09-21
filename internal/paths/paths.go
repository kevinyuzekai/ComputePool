package paths

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultInboxName  = "ComputePool-Inbox"
	DefaultOutboxName = "ComputePool-Outbox"
)

// Expand expands leading ~ to the user home directory.
func Expand(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return p
	}
	if p == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return home
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return filepath.Join(home, p[2:])
	}
	return p
}

// DefaultInbox returns ~/ComputePool-Inbox.
func DefaultInbox() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", DefaultInboxName)
	}
	return filepath.Join(home, DefaultInboxName)
}

// DefaultOutbox returns ~/ComputePool-Outbox.
func DefaultOutbox() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", DefaultOutboxName)
	}
	return filepath.Join(home, DefaultOutboxName)
}

// EnsureDir creates dir if missing (mkdir -p).
func EnsureDir(dir string) error {
	dir = Expand(dir)
	if dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

// JobOutbox returns outboxDir/<jobId>.
func JobOutbox(outboxDir, jobID string) string {
	return filepath.Join(Expand(outboxDir), jobID)
}
