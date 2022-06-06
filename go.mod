module github.com/yacchi/otelchi

go 1.16

require (
	github.com/felixge/httpsnoop v1.0.3
	github.com/go-chi/chi/v5 v5.0.7
	github.com/stretchr/testify v1.7.1
	go.opentelemetry.io/contrib v1.7.0
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.32.0
	go.opentelemetry.io/contrib/propagators/b3 v1.4.0
	go.opentelemetry.io/otel v1.7.0
	go.opentelemetry.io/otel/oteltest v1.0.0-RC3
	go.opentelemetry.io/otel/trace v1.7.0
)
