package integration

import (
	"context"
	"fmt"
	"net/http"

	"github.com/felixge/httpsnoop"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func TelemetryGlobalMiddleware(next http.Handler) http.Handler {
	return otelhttp.NewHandler(next, "server", otelhttp.WithMessageEvents(otelhttp.ReadEvents, otelhttp.WriteEvents))
}

func AccessLogsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := httpsnoop.CaptureMetrics(next, w, r)
		logHTTPRequest(r, m)
	})
}

func RecoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				err := fmt.Errorf("%+v", err)
				HandleHTTPServerError(w, r, err)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type RequestURIKey struct{}

func RequestURIMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		ctx = context.WithValue(ctx, RequestURIKey{}, r.RequestURI)
		r = r.WithContext(ctx)
		next.ServeHTTP(w, r)
	})
}
