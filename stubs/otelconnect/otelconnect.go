// Package otelconnect is a minimal stub for Go 1.23 compatibility.
// The real module requires Go 1.24; this stub allows the build to succeed.
package otelconnect

// Option configures NewInterceptor.
type Option interface{}

// Interceptor is a no-op stub for OpenTelemetry interceptors.
type Interceptor struct{}

// NewInterceptor returns a stub interceptor (no-op).
func NewInterceptor(_ ...Option) (*Interceptor, error) {
	return &Interceptor{}, nil
}
