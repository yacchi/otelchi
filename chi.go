package otelchi

import (
	"fmt"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"net/http"
	"sync"

	"github.com/felixge/httpsnoop"
	"github.com/go-chi/chi/v5"

	otelcontrib "go.opentelemetry.io/contrib"
	"go.opentelemetry.io/otel"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/semconv/v1.7.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const (
	tracerName = "github.com/yacchi/otelchi"
)

// Middleware sets up a handler to start tracing the incoming
// requests.  The service parameter should describe the name of the
// (virtual) server handling the request.
func Middleware(service string, opts ...Option) func(http.Handler) http.Handler {
	cfg := config{}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.TracerProvider == nil {
		cfg.TracerProvider = otel.GetTracerProvider()
	}
	tracer := cfg.TracerProvider.Tracer(
		tracerName,
		oteltrace.WithInstrumentationVersion(otelcontrib.SemVersion()),
	)
	if cfg.Propagators == nil {
		cfg.Propagators = otel.GetTextMapPropagator()
	}
	return func(handler http.Handler) http.Handler {
		return traceware{
			service:     service,
			tracer:      tracer,
			propagators: cfg.Propagators,
			handler:     handler,
			filters:     cfg.Filters,
		}
	}
}

type traceware struct {
	service     string
	tracer      oteltrace.Tracer
	propagators propagation.TextMapPropagator
	handler     http.Handler
	filters     []otelhttp.Filter
}

type recordingResponseWriter struct {
	writer  http.ResponseWriter
	written bool
	status  int
}

var rrwPool = &sync.Pool{
	New: func() interface{} {
		return &recordingResponseWriter{}
	},
}

func getRRW(writer http.ResponseWriter) *recordingResponseWriter {
	rrw := rrwPool.Get().(*recordingResponseWriter)
	rrw.written = false
	rrw.status = 0
	rrw.writer = httpsnoop.Wrap(writer, httpsnoop.Hooks{
		Write: func(next httpsnoop.WriteFunc) httpsnoop.WriteFunc {
			return func(b []byte) (int, error) {
				if !rrw.written {
					rrw.written = true
					rrw.status = http.StatusOK
				}
				return next(b)
			}
		},
		WriteHeader: func(next httpsnoop.WriteHeaderFunc) httpsnoop.WriteHeaderFunc {
			return func(statusCode int) {
				if !rrw.written {
					rrw.written = true
					rrw.status = statusCode
				}
				next(statusCode)
			}
		},
	})
	return rrw
}

func putRRW(rrw *recordingResponseWriter) {
	rrw.writer = nil
	rrwPool.Put(rrw)
}

// ServeHTTP implements the http.Handler interface. It does the actual
// tracing of the request.
func (t traceware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	for _, f := range t.filters {
		if !f(r) {
			// Simply pass through to the handler if a filter rejects the request
			t.handler.ServeHTTP(w, r)
			return
		}
	}
	ctx := t.propagators.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
	spanName := ""
	route := chi.RouteContext(ctx)
	if route != nil {
		spanName = route.RoutePattern()
	}
	routeStr := spanName
	if spanName == "" {
		spanName = fmt.Sprintf("HTTP %s route not found", r.Method)
	}
	opts := []oteltrace.SpanStartOption{
		oteltrace.WithAttributes(semconv.NetAttributesFromHTTPRequest("tcp", r)...),
		oteltrace.WithAttributes(semconv.EndUserAttributesFromHTTPRequest(r)...),
		oteltrace.WithAttributes(semconv.HTTPServerAttributesFromHTTPRequest(t.service, routeStr, r)...),
		oteltrace.WithSpanKind(oteltrace.SpanKindServer),
	}
	ctx, span := t.tracer.Start(ctx, spanName, opts...)
	defer span.End()
	r2 := r.WithContext(ctx)
	rrw := getRRW(w)
	defer putRRW(rrw)
	t.handler.ServeHTTP(rrw.writer, r2)
	attrs := semconv.HTTPAttributesFromHTTPStatusCode(rrw.status)
	spanStatus, spanMessage := semconv.SpanStatusFromHTTPStatusCode(rrw.status)
	span.SetAttributes(attrs...)
	span.SetStatus(spanStatus, spanMessage)
}
