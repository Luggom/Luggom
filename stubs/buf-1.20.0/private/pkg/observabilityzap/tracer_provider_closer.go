// Copyright 2020-2023 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package observabilityzap

import (
	"context"
	"io"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

var _ trace.TracerProvider = &tracerProviderCloser{}
var _ io.Closer = &tracerProviderCloser{}

// Embed noop.TracerProvider so we satisfy the otel trace.TracerProvider interface
// (including the unexported tracerProvider() method in otel v1.35+).
type tracerProviderCloser struct {
	noop.TracerProvider
	inner *sdktrace.TracerProvider
}

func newTracerProviderCloser(inner *sdktrace.TracerProvider) *tracerProviderCloser {
	return &tracerProviderCloser{inner: inner}
}

func (t *tracerProviderCloser) Tracer(name string, opts ...trace.TracerOption) trace.Tracer {
	return t.inner.Tracer(name, opts...)
}

func (t *tracerProviderCloser) Close() error {
	return t.inner.Shutdown(context.Background())
}
