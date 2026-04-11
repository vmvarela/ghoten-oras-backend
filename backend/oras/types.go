package oras

import (
	"context"
	"crypto/md5"
	"fmt"
	"time"
)

// StatePayload represents a state file retrieved from the registry.
// MD5 is used for ETag-like change detection, not for security.
type StatePayload struct {
	Data []byte
	MD5  []byte
}

// ComputeMD5 returns a new StatePayload with the MD5 checksum computed
// from Data. Useful when constructing payloads from raw bytes.
func ComputeMD5(data []byte) StatePayload {
	sum := md5.Sum(data)
	return StatePayload{Data: data, MD5: sum[:]}
}

// LockInfo records metadata about a state lock holder. It mirrors
// the fields commonly stored by Terraform/OpenTofu backends.
type LockInfo struct {
	ID        string    `json:"ID"`
	Operation string    `json:"Operation"`
	Info      string    `json:"Info"`
	Who       string    `json:"Who"`
	Version   string    `json:"Version"`
	Created   time.Time `json:"Created"`
	Path      string    `json:"Path"`
}

// LockError represents a failed lock acquisition, optionally carrying
// the lock holder's information.
type LockError struct {
	Info             *LockInfo
	Err              error
	InconsistentRead bool
}

func (e *LockError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return "lock error"
}

func (e *LockError) Unwrap() error {
	return e.Err
}

// DefaultStateName is the canonical name of the default workspace.
const DefaultStateName = "default"

// StateBackend is the primary interface of this library. It provides
// workspace-level state management over an OCI registry via ORAS.
type StateBackend interface {
	// Workspaces returns the list of known workspaces. The default
	// workspace is always included even when no state has been written.
	Workspaces(ctx context.Context) ([]string, error)

	// DeleteWorkspace removes all artifacts for the named workspace
	// (state, lock, unlocked, version tags). Deleting the default
	// workspace returns an error.
	DeleteWorkspace(ctx context.Context, name string, force bool) error

	// StateMgr returns a StateMgr scoped to the given workspace.
	StateMgr(ctx context.Context, workspace string) (StateMgr, error)

	// Close releases resources held by the backend (e.g. HTTP clients).
	Close() error
}

// StateMgr provides read/write/lock operations on a single workspace's state.
type StateMgr interface {
	// Get retrieves the current state. Returns nil when no state exists.
	Get(ctx context.Context) (*StatePayload, error)

	// Put writes state data to the registry.
	Put(ctx context.Context, data []byte) error

	// Delete removes the state from the registry.
	Delete(ctx context.Context) error

	// Lock acquires a workspace lock. Returns the lock ID on success.
	Lock(ctx context.Context, info *LockInfo) (string, error)

	// Unlock releases a workspace lock.
	Unlock(ctx context.Context, id string) error

	// WaitForRetention blocks until all in-flight async retention
	// goroutines have completed. Call before process exit when
	// versioning is enabled.
	WaitForRetention()
}

// Config holds all configuration for creating an ORAS StateBackend.
type Config struct {
	// Repository is the OCI repository in the form <registry>/<repository>,
	// without a tag or digest. Required.
	Repository string

	// Insecure skips TLS certificate verification. Default: false.
	Insecure bool

	// CAFile is the path to a PEM-encoded CA certificate bundle to trust.
	CAFile string

	// Compression controls state compression. Supported: "none", "gzip".
	// Default: "none".
	Compression string

	// LockTTL is the lock time-to-live. When greater than zero, stale locks
	// older than this duration are automatically cleared during lock
	// acquisition. Default: 0 (no automatic clearing).
	LockTTL time.Duration

	// RateLimit is the maximum registry requests per second. 0 disables.
	RateLimit int

	// RateBurst is the maximum burst size for rate limiting. 0 defaults to 1.
	RateBurst int

	// Retry configures retry behavior for transient registry errors.
	// A zero value uses DefaultRetryConfig().
	Retry RetryConfig

	// MaxStateSize is the maximum number of bytes to read from a state layer.
	// 0 uses the built-in default (256 MiB).
	MaxStateSize int64

	// MaxVersions controls state versioning: 0 disables, >0 enables with
	// that retention limit.
	MaxVersions int

	// UserAgent is the HTTP User-Agent string sent with registry requests.
	// If empty, a default value is used.
	UserAgent string

	// CredentialFunc returns OCI registry credentials for a given host.
	// If nil, requests are made without authentication.
	CredentialFunc CredentialFunc
}

func (c *Config) validate() error {
	if c.Repository == "" {
		return fmt.Errorf("repository must not be empty")
	}
	switch c.Compression {
	case "", "none", "gzip":
	default:
		return fmt.Errorf("unsupported compression %q (supported: none, gzip)", c.Compression)
	}
	if c.LockTTL < 0 {
		return fmt.Errorf("lock_ttl must be non-negative")
	}
	if c.RateLimit < 0 {
		return fmt.Errorf("rate_limit must be non-negative")
	}
	if c.RateBurst < 0 {
		return fmt.Errorf("rate_limit_burst must be non-negative")
	}
	if c.MaxStateSize < 0 {
		return fmt.Errorf("max_state_size must be non-negative")
	}
	if c.MaxVersions < 0 {
		return fmt.Errorf("max_versions must be non-negative")
	}
	return nil
}

// CredentialFunc returns registry credentials for the given host.
// It follows the oras-go auth.CredentialFunc signature.
type CredentialFunc func(ctx context.Context, hostport string) (Credential, error)

// Credential holds username/password or token credentials for OCI auth.
type Credential struct {
	Username    string
	Password    string
	AccessToken string
}
