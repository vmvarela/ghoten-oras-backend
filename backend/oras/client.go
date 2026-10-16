package oras

import (
	"context"
	"crypto/md5"
	"errors"

	state "github.com/vmvarela/ghoten-oras-backend/state"
)

// Client adapts the reusable state.Client to the OpenTofu-style StateMgr
// interface exposed by this package. Registry semantics live in package state.
type Client struct {
	client        *state.Client
	workspaceName string
	retryConfig   RetryConfig
}

var _ StateMgr = (*Client)(nil)

func newClient(client *state.Client, workspaceName string, retryCfg RetryConfig) *Client {
	if retryCfg == (RetryConfig{}) {
		retryCfg = DefaultRetryConfig()
	}
	return &Client{
		client:        client,
		workspaceName: workspaceName,
		retryConfig:   retryCfg,
	}
}

func (c *Client) Get(ctx context.Context) (*StatePayload, error) {
	return withRetry(ctx, c.retryConfig, func(ctx context.Context) (*StatePayload, error) {
		data, err := c.client.Get(ctx, c.workspaceName)
		if err != nil {
			return nil, err
		}
		if data == nil {
			return nil, nil
		}
		sum := md5.Sum(data) // StatePayload compatibility checksum, not security.
		return &StatePayload{Data: data, MD5: sum[:]}, nil
	})
}

func (c *Client) Put(ctx context.Context, data []byte) error {
	if err := withRetryNoResult(ctx, c.retryConfig, func(ctx context.Context) error {
		return c.client.Put(ctx, c.workspaceName, data)
	}); err != nil {
		return err
	}

	// Keep asynchronous retention inside the reusable engine, but make the
	// OpenTofu adapter synchronous at its boundary. This avoids leaking a
	// backend-specific "wait for retention" capability into OpenTofu Core.
	c.client.WaitForRetention()
	return nil
}

func (c *Client) Delete(ctx context.Context) error {
	return withRetryNoResult(ctx, c.retryConfig, func(ctx context.Context) error {
		return c.client.Delete(ctx, c.workspaceName)
	})
}

func (c *Client) Lock(ctx context.Context, info *LockInfo) (string, error) {
	if info == nil {
		info = &LockInfo{}
	}
	id, err := withRetry(ctx, c.retryConfig, func(ctx context.Context) (string, error) {
		return c.client.Lock(ctx, c.workspaceName, state.LockInfo{
			ID:        info.ID,
			Operation: info.Operation,
			Info:      info.Info,
			Who:       info.Who,
			Version:   info.Version,
			Created:   info.Created,
			Path:      info.Path,
		})
	})
	if err == nil {
		return id, nil
	}

	var sharedErr *state.LockError
	if errors.As(err, &sharedErr) {
		var lockInfo *LockInfo
		if sharedErr.Info != nil {
			lockInfo = &LockInfo{
				ID:        sharedErr.Info.ID,
				Operation: sharedErr.Info.Operation,
				Info:      sharedErr.Info.Info,
				Who:       sharedErr.Info.Who,
				Version:   sharedErr.Info.Version,
				Created:   sharedErr.Info.Created,
				Path:      sharedErr.Info.Path,
			}
		}
		return "", &LockError{Info: lockInfo, Err: sharedErr.Err}
	}
	return "", err
}

func (c *Client) Unlock(ctx context.Context, id string) error {
	return withRetryNoResult(ctx, c.retryConfig, func(ctx context.Context) error {
		return c.client.Unlock(ctx, c.workspaceName, id)
	})
}

func (c *Client) WaitForRetention() {
	c.client.WaitForRetention()
}
