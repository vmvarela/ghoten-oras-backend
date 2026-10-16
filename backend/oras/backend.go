package oras

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	state "github.com/vmvarela/ghoten-oras-backend/state"
	orasRegistry "oras.land/oras-go/v2/registry"
)

// ORASBackend adapts the reusable state engine to the backend interfaces used
// by OpenTofu/Ghoten. OCI behavior itself belongs to package state.
type ORASBackend struct {
	client *state.Client
	config Config
}

var _ StateBackend = (*ORASBackend)(nil)

func New(_ context.Context, cfg Config) (StateBackend, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	ref, err := orasRegistry.ParseReference(cfg.Repository)
	if err != nil {
		return nil, fmt.Errorf("invalid OCI repository %q: %w", cfg.Repository, err)
	}
	if ref.Reference != "" {
		return nil, fmt.Errorf("repository must not include a tag or digest")
	}

	httpClient, err := newHTTPClient(cfg, slog.Default())
	if err != nil {
		return nil, err
	}

	sharedCfg := state.Config{
		WorkspaceLayout: state.WorkspaceLayoutLegacyGhoten,
		PlainHTTP:       cfg.Insecure,
		TLSSkipVerify:   cfg.Insecure,
		CAFile:          cfg.CAFile,
		Compression:     cfg.Compression == "gzip",
		LockTTL:         cfg.LockTTL,
		MaxVersions:     cfg.MaxVersions,
		MaxStateSize:    cfg.MaxStateSize,
		HTTPClient:      httpClient,
		UserAgent:       cfg.UserAgent,
	}
	if cfg.CredentialFunc != nil {
		sharedCfg.CredentialFunc = func(ctx context.Context, hostport string) (state.Credential, error) {
			cred, err := cfg.CredentialFunc(ctx, hostport)
			if err != nil {
				return state.Credential{}, err
			}
			return state.Credential{
				Username:    cred.Username,
				Password:    cred.Password,
				AccessToken: cred.AccessToken,
			}, nil
		}
	}

	client, err := state.NewClient(ref.Registry, ref.Repository, sharedCfg)
	if err != nil {
		return nil, err
	}

	return &ORASBackend{client: client, config: cfg}, nil
}

func (b *ORASBackend) StateMgr(_ context.Context, workspace string) (StateMgr, error) {
	return newClient(b.client, workspace, b.config.Retry), nil
}

func (b *ORASBackend) Workspaces(ctx context.Context) ([]string, error) {
	workspaces, err := b.client.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, workspace := range workspaces {
		if workspace == DefaultStateName {
			return workspaces, nil
		}
	}
	workspaces = append(workspaces, DefaultStateName)
	sort.Strings(workspaces)
	return workspaces, nil
}

func (b *ORASBackend) DeleteWorkspace(ctx context.Context, name string, _ bool) error {
	if name == DefaultStateName || name == "" {
		return fmt.Errorf("can't delete default state")
	}
	cfg := b.config.Retry
	if cfg == (RetryConfig{}) {
		cfg = DefaultRetryConfig()
	}
	return withRetryNoResult(ctx, cfg, func(ctx context.Context) error {
		return b.client.DeleteWorkspace(ctx, name)
	})
}

func (b *ORASBackend) Close() error {
	return b.client.Close()
}
