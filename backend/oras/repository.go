package oras

import (
	"context"
	"io"
	"net/http"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// orasRepository is the minimal subset of the ORAS repository interface
// used by this package. It exists to allow unit testing with in-memory fakes.
type orasRepository interface {
	Push(ctx context.Context, expected ocispec.Descriptor, content io.Reader) error
	Fetch(ctx context.Context, target ocispec.Descriptor) (io.ReadCloser, error)
	Resolve(ctx context.Context, reference string) (ocispec.Descriptor, error)
	Tag(ctx context.Context, desc ocispec.Descriptor, reference string) error
	Delete(ctx context.Context, target ocispec.Descriptor) error
	Tags(ctx context.Context, last string, fn func(tags []string) error) error
}

// repositoryClient wraps an orasRepository with authentication and HTTP client
// state needed for GHCR API calls and token resolution.
type repositoryClient struct {
	repository string
	inner      orasRepository
	authFn     CredentialFunc
	httpClient httpClient
}

// httpClient abstracts *http.Client for testing.
type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

func (r *repositoryClient) accessTokenForHost(ctx context.Context, host string) (string, error) {
	if r == nil || r.authFn == nil {
		return "", nil
	}
	cred, err := r.authFn(ctx, host)
	if err != nil {
		return "", err
	}
	if cred.AccessToken != "" {
		return cred.AccessToken, nil
	}
	if cred.Password != "" {
		return cred.Password, nil
	}
	return "", nil
}
