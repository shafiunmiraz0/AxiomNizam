package observability

import (
	"context"
	"os"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.uber.org/zap"
)

// InitTracer sets up the OpenTelemetry TracerProvider.
//
// If OTEL_EXPORTER_OTLP_ENDPOINT is set, traces are exported to the
// configured OTLP gRPC collector.  Otherwise a no-op provider is
// installed so all instrumentation code works without an exporter.
//
// Call the returned function at process shutdown to flush pending spans.
func InitTracer(ctx context.Context, serviceName string) (shutdown func(context.Context) error, err error) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		// No exporter configured — install a no-op provider.
		// OTel API calls become zero-cost no-ops.
		logging.Z().Info("OTel tracing disabled (no OTEL_EXPORTER_OTLP_ENDPOINT)")
		return func(context.Context) error { return nil }, nil
	}

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(), // TLS configured via OTEL_EXPORTER_OTLP_CERTIFICATE
	)
	if err != nil {
		return nil, err
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
			attribute.String("service.version", serviceVersion()),
		),
	)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(parentBasedSampler()),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	logging.Z().Info("OTel tracing enabled",
		zap.String("endpoint", endpoint),
		zap.String("service", serviceName),
	)

	return tp.Shutdown, nil
}

// parentBasedSampler returns a sampler that respects the parent span's
// sampling decision and samples root spans at 100%.
func parentBasedSampler() sdktrace.Sampler {
	return sdktrace.ParentBased(sdktrace.AlwaysSample())
}

// serviceVersion reads AXIOM_VERSION env or defaults to "dev".
func serviceVersion() string {
	if v := os.Getenv("AXIOM_VERSION"); v != "" {
		return v
	}
	return "dev"
}
