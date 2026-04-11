package oras

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	cleanhttp "github.com/hashicorp/go-cleanhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/time/rate"
)

const defaultUserAgent = "ghoten-oras-backend"

// newHTTPClient builds an *http.Client configured with the given Config
// settings (TLS, rate limiting, user agent, OpenTelemetry tracing).
func newHTTPClient(cfg Config, logger *slog.Logger) (*http.Client, error) {
	client := cleanhttp.DefaultPooledClient()

	if t, ok := client.Transport.(*http.Transport); ok {
		t = t.Clone()
		if t.TLSClientConfig == nil {
			t.TLSClientConfig = &tls.Config{}
		}
		t.TLSClientConfig.InsecureSkipVerify = cfg.Insecure
		if cfg.Insecure {
			logger.Warn("TLS certificate verification is disabled", "insecure", true)
		}
		if cfg.CAFile != "" {
			pem, err := os.ReadFile(cfg.CAFile)
			if err != nil {
				return nil, fmt.Errorf("reading ca_file %q: %w", cfg.CAFile, err)
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(pem) {
				return nil, fmt.Errorf("ca_file %q: no valid certificates", cfg.CAFile)
			}
			t.TLSClientConfig.RootCAs = pool
		}
		client.Transport = t
	}

	var limiter requestLimiter
	if cfg.RateLimit > 0 {
		burst := cfg.RateBurst
		if burst <= 0 {
			burst = 1
		}
		limiter = rate.NewLimiter(rate.Limit(cfg.RateLimit), burst)
	}

	ua := cfg.UserAgent
	if ua == "" {
		ua = defaultUserAgent
	}

	var rt http.RoundTripper = &userAgentRoundTripper{
		userAgent: ua,
		inner:     client.Transport,
	}
	if limiter != nil {
		rt = &rateLimitedRoundTripper{limiter: limiter, inner: rt}
	}
	// Always wrap with otelhttp so that spans created by later operations
	// are captured. otelhttp is a no-op when there is no active span,
	// so there is no overhead when tracing is disabled.
	rt = otelhttp.NewTransport(rt)
	client.Transport = rt

	return client, nil
}

// requestLimiter abstracts *rate.Limiter for testing.
type requestLimiter interface {
	Wait(ctx context.Context) error
}

type userAgentRoundTripper struct {
	userAgent string
	inner     http.RoundTripper
}

// RoundTrip sets the User-Agent header if not already present.
// Per the http.RoundTripper contract, the original request is not mutated;
// a shallow clone is created when the header needs to be added.
func (rt *userAgentRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		r2 := req.Clone(req.Context())
		r2.Header.Set("User-Agent", rt.userAgent)
		return rt.inner.RoundTrip(r2)
	}
	return rt.inner.RoundTrip(req)
}

type rateLimitedRoundTripper struct {
	limiter requestLimiter
	inner   http.RoundTripper
}

func (rt *rateLimitedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if rt.limiter != nil {
		if err := rt.limiter.Wait(req.Context()); err != nil {
			return nil, err
		}
	}
	return rt.inner.RoundTrip(req)
}
