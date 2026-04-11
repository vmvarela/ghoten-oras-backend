# ghoten-oras-backend

A standalone Go library for storing Terraform/OpenTofu state as OCI artifacts in container registries using the [ORAS](https://oras.land/) protocol.

## Features

- **Provider-agnostic** — works with both Terraform and OpenTofu
- **Registry support** — GHCR, Zot, and any OCI-compliant registry
- **Workspace management** — list, create, delete workspaces via manifest tags
- **State locking** — generation-based optimistic locking with configurable TTL
- **Versioning** — configurable state version retention with async cleanup
- **Compression** — optional gzip compression for state payloads
- **GHCR fallback** — handles GHCR's lack of tag deletion via GitHub API
- **Retry & rate limiting** — built-in exponential backoff and per-second rate limiting
- **OpenTelemetry** — HTTP transport instrumented with otelhttp
- **No encryption** — raw `[]byte` in/out; encryption belongs in the caller

## Installation

```bash
go get github.com/vmvarela/ghoten-oras-backend
```

Requires **Go 1.25+**.

## Quick Start

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/vmvarela/ghoten-oras-backend/backend/oras"
)

func main() {
	ctx := context.Background()

	backend, err := oras.New(ctx, oras.Config{
		Repository: "ghcr.io/myorg/myrepo",
		CredentialFunc: func(ctx context.Context, host string) (oras.Credential, error) {
			return oras.Credential{
				Username: "myuser",
				Password: "mytoken",
			}, nil
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer backend.Close()

	// List workspaces
	workspaces, err := backend.Workspaces(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Workspaces:", workspaces)

	// Get a state manager for the default workspace
	mgr, err := backend.StateMgr(ctx, oras.DefaultStateName)
	if err != nil {
		log.Fatal(err)
	}

	// Read current state
	payload, err := mgr.Get(ctx)
	if err != nil {
		log.Fatal(err)
	}
	if payload != nil {
		fmt.Printf("State size: %d bytes\n", len(payload.Data))
	}
}
```

## API

### `oras.New`

```go
func New(ctx context.Context, cfg Config) (StateBackend, error)
```

Creates a new ORAS backend. Validates the config and establishes the OCI repository connection.

### `StateBackend`

```go
type StateBackend interface {
    Workspaces(ctx context.Context) ([]string, error)
    DeleteWorkspace(ctx context.Context, name string, force bool) error
    StateMgr(ctx context.Context, workspace string) (StateMgr, error)
    Close() error
}
```

### `StateMgr`

```go
type StateMgr interface {
    Get(ctx context.Context) (*StatePayload, error)
    Put(ctx context.Context, data []byte) error
    Delete(ctx context.Context) error
    Lock(ctx context.Context, info *LockInfo) (string, error)
    Unlock(ctx context.Context, id string) error
    WaitForRetention()
}
```

### `Config`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `Repository` | `string` | — | OCI repository (e.g. `ghcr.io/org/repo`). **Required.** |
| `Insecure` | `bool` | `false` | Skip TLS verification. |
| `CAFile` | `string` | `""` | Path to PEM CA certificate bundle. |
| `Compression` | `string` | `"none"` | `"none"` or `"gzip"`. |
| `LockTTL` | `time.Duration` | `0` | Auto-clear stale locks older than this. |
| `RateLimit` | `int` | `0` | Max registry requests/second (0 = unlimited). |
| `RateBurst` | `int` | `0` | Rate limiter burst size. |
| `Retry` | `RetryConfig` | 3 attempts, 1s–30s | Retry config for transient errors. |
| `MaxStateSize` | `int64` | `256 MiB` | Max state payload size. |
| `MaxVersions` | `int` | `0` | State version retention (0 = disabled). |
| `UserAgent` | `string` | `""` | Custom HTTP User-Agent. |
| `CredentialFunc` | `CredentialFunc` | `nil` | Returns credentials per registry host. |

### `RetryConfig`

```go
type RetryConfig struct {
    MaxAttempts       int           // Default: 3
    InitialBackoff    time.Duration // Default: 1s
    MaxBackoff        time.Duration // Default: 30s
    BackoffMultiplier float64       // Default: 2.0
}
```

### Types

```go
type StatePayload struct {
    Data []byte
    MD5  []byte
}

type LockInfo struct {
    ID        string
    Operation string
    Info      string
    Who       string
    Version   string
    Created   time.Time
    Path      string
}

type LockError struct {
    Info             *LockInfo
    Err              error
    InconsistentRead bool
}

type Credential struct {
    Username    string
    Password    string
    AccessToken string
}

type CredentialFunc func(ctx context.Context, hostport string) (Credential, error)
```

## Locking

State locking uses generation-based optimistic concurrency:

1. **Lock** pushes a manifest with `LockInfo` as a JSON annotation, tagged `lock-<workspace>`.
2. The manifest includes the current generation (resolved from the existing lock tag). If the tag moved since resolution, the push fails — another holder acquired the lock.
3. **Unlock** deletes the lock manifest (or tags an "unlocked" sentinel on GHCR where tag deletion is unsupported).
4. **LockTTL** — when set, stale locks older than the TTL are automatically cleared during lock acquisition.

## Versioning

When `MaxVersions > 0`, each `Put` creates a versioned snapshot tagged `state-<workspace>-v<N>`. A background goroutine prunes versions exceeding the retention limit. Call `WaitForRetention()` before process exit to ensure cleanup completes.

## GHCR Compatibility

GitHub Container Registry doesn't support the OCI tag deletion API. This library includes a fallback that uses the GitHub Packages REST API to delete package versions by tag. The fallback activates automatically when the registry host is `ghcr.io`.

## License

See [LICENSE](LICENSE).
