package oras

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	orasRemote "oras.land/oras-go/v2/registry/remote"
	orasAuth "oras.land/oras-go/v2/registry/remote/auth"
)

// ORASBackend implements StateBackend for OCI registries using the ORAS
// protocol. It stores state as OCI artifacts and supports workspace
// management via manifest tags.
type ORASBackend struct {
	repoClient *repositoryClient
	config     Config
	logger     *slog.Logger
}

// Compile-time check that ORASBackend implements StateBackend.
var _ StateBackend = (*ORASBackend)(nil)

// New creates a new ORAS StateBackend from the given Config.
// The Config is validated before use; invalid configs return an error.
func New(ctx context.Context, cfg Config) (StateBackend, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	logger := slog.Default()

	repoClient, err := newORASRepositoryClient(ctx, cfg, logger)
	if err != nil {
		return nil, err
	}

	return &ORASBackend{
		repoClient: repoClient,
		config:     cfg,
		logger:     logger,
	}, nil
}

func newORASRepositoryClient(ctx context.Context, cfg Config, logger *slog.Logger) (*repositoryClient, error) {
	repo, err := orasRemote.NewRepository(cfg.Repository)
	if err != nil {
		return nil, fmt.Errorf("invalid OCI repository %q: %w", cfg.Repository, err)
	}

	if cfg.Insecure {
		repo.PlainHTTP = true
	}

	httpCl, err := newHTTPClient(cfg, logger)
	if err != nil {
		return nil, err
	}

	repoClient := &repositoryClient{
		repository: cfg.Repository,
		inner:      repo,
		authFn:     cfg.CredentialFunc,
		httpClient: httpCl,
	}

	// Configure ORAS auth client on the remote repository.
	if cfg.CredentialFunc != nil {
		repo.Client = &orasAuth.Client{
			Client: httpCl,
			Credential: func(ctx context.Context, host string) (orasAuth.Credential, error) {
				cred, err := cfg.CredentialFunc(ctx, host)
				if err != nil {
					return orasAuth.EmptyCredential, err
				}
				return orasAuth.Credential{
					Username:    cred.Username,
					Password:    cred.Password,
					AccessToken: cred.AccessToken,
				}, nil
			},
		}
	} else {
		repo.Client = &orasAuth.Client{
			Client: httpCl,
		}
	}

	return repoClient, nil
}

func (b *ORASBackend) StateMgr(_ context.Context, workspace string) (StateMgr, error) {
	client := newClient(b.repoClient, workspace, b.logger)
	client.retryConfig = b.config.Retry
	if client.retryConfig == (RetryConfig{}) {
		client.retryConfig = DefaultRetryConfig()
	}
	client.versioningMaxVersions = b.config.MaxVersions
	client.stateCompression = b.config.Compression
	if client.stateCompression == "" {
		client.stateCompression = "none"
	}
	client.stateMaxSize = b.config.MaxStateSize
	client.lockTTL = b.config.LockTTL
	return client, nil
}

func (b *ORASBackend) Workspaces(ctx context.Context) ([]string, error) {
	wss, err := listWorkspacesFromTags(ctx, b.repoClient)
	if err != nil {
		if isNotFound(err) {
			return []string{DefaultStateName}, nil
		}
		return nil, err
	}

	// The backend contract requires that the default workspace is always
	// present in the returned list, even if no state has been written yet.
	hasDefault := false
	for _, w := range wss {
		if w == DefaultStateName {
			hasDefault = true
			break
		}
	}
	if !hasDefault {
		wss = append(wss, DefaultStateName)
		sort.Strings(wss)
	}
	return wss, nil
}

func (b *ORASBackend) DeleteWorkspace(ctx context.Context, name string, _ bool) error {
	if name == DefaultStateName || name == "" {
		return fmt.Errorf("can't delete default state")
	}

	wsTag := workspaceTagFor(name)
	stateRef := stateTagPrefix + wsTag
	lockRef := lockTagPrefix + wsTag
	unlockedRef := unlockedTagPrefix + wsTag
	stateVersionPrefix := stateRef + stateVersionTagSeparator

	// Delete the main state manifest.
	if err := resolveAndDelete(ctx, b.repoClient, stateRef); err != nil {
		return fmt.Errorf("deleting state for workspace %q: %w", name, err)
	}

	// Delete versioned state manifests.
	if err := b.repoClient.inner.Tags(ctx, "", func(page []string) error {
		for _, tag := range page {
			if !strings.HasPrefix(tag, stateVersionPrefix) {
				continue
			}
			if err := resolveAndDelete(ctx, b.repoClient, tag); err != nil {
				return fmt.Errorf("deleting state version %q for workspace %q: %w", tag, name, err)
			}
		}
		return nil
	}); err != nil {
		return err
	}

	// Delete the lock manifest.
	if err := resolveAndDelete(ctx, b.repoClient, lockRef); err != nil {
		return fmt.Errorf("deleting lock for workspace %q: %w", name, err)
	}

	// Delete the unlocked manifest (GHCR fallback artifact).
	if err := resolveAndDelete(ctx, b.repoClient, unlockedRef); err != nil {
		return fmt.Errorf("deleting unlocked tag for workspace %q: %w", name, err)
	}
	return nil
}

// resolveAndDelete resolves a tag and deletes the underlying manifest.
// Returns nil when the tag doesn't exist (404). Transient Resolve errors
// are surfaced instead of silently swallowed.
func resolveAndDelete(ctx context.Context, repo *repositoryClient, ref string) error {
	desc, err := repo.inner.Resolve(ctx, ref)
	if err != nil {
		if isNotFound(err) {
			return nil
		}
		return err
	}
	if err := repo.inner.Delete(ctx, desc); err != nil && !isNotFound(err) && !isDeleteUnsupported(err) {
		return err
	}
	return nil
}

func (b *ORASBackend) Close() error {
	// Currently no resources to release.
	return nil
}
