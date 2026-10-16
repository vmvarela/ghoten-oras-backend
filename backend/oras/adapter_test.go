package oras

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestBackendConfigValidation(t *testing.T) {
	if _, err := New(context.Background(), Config{}); err == nil {
		t.Fatal("empty repository accepted")
	}
	if _, err := New(context.Background(), Config{Repository: "ghcr.io/acme/state:latest"}); err == nil {
		t.Fatal("repository tag accepted")
	}
	if _, err := New(context.Background(), Config{Repository: "ghcr.io/acme/state", Compression: "brotli"}); err == nil {
		t.Fatal("unsupported compression accepted")
	}
}

func TestRetryCompatibility(t *testing.T) {
	cfg := RetryConfig{
		MaxAttempts:       3,
		InitialBackoff:    time.Millisecond,
		MaxBackoff:        time.Millisecond,
		BackoffMultiplier: 1,
	}
	attempts := 0
	got, err := withRetry(context.Background(), cfg, func(context.Context) (string, error) {
		attempts++
		if attempts < 3 {
			return "", timeoutError{}
		}
		return "ok", nil
	})
	if err != nil || got != "ok" || attempts != 3 {
		t.Fatalf("got %q, %v after %d attempts", got, err, attempts)
	}

	sentinel := errors.New("permanent")
	attempts = 0
	_, err = withRetry(context.Background(), cfg, func(context.Context) (string, error) {
		attempts++
		return "", sentinel
	})
	if !errors.Is(err, sentinel) || attempts != 1 {
		t.Fatalf("non-transient error retried: %v after %d attempts", err, attempts)
	}
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

var _ net.Error = timeoutError{}
