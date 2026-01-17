package integration

import (
	"crypto/tls"
	"net/http"
	"time"

	"github.com/hashicorp/go-cleanhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func NewHTTPClient(serverName string, middlewares ...HTTPClientMiddleware) *http.Client {
	return NewHTTPClientWithTLS(serverName, nil, middlewares...)
}

func NewHTTPClientWithTLS(serverName string, tlsCfg *tls.Config, middlewares ...HTTPClientMiddleware) *http.Client {
	pooledTransport := cleanhttp.DefaultPooledTransport()
	pooledTransport.TLSClientConfig = tlsCfg
	var transport http.RoundTripper = otelhttp.NewTransport(
		pooledTransport,
		otelhttp.WithServerName(serverName),
	)
	for _, middleware := range middlewares {
		transport = &middlewareRoundTripper{
			roundTripper: transport,
			middleware:   middleware,
		}
	}
	return &http.Client{Timeout: time.Minute, Transport: transport}
}

type HTTPClientMiddleware func(req *http.Request, next http.RoundTripper) (*http.Response, error)

type middlewareRoundTripper struct {
	roundTripper http.RoundTripper
	middleware   func(req *http.Request, next http.RoundTripper) (*http.Response, error)
}

func (t *middlewareRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.middleware(req, t.roundTripper)
}
