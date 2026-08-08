package main

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// setupTracing initializes the global TracerProvider with an OTLP gRPC
// exporter pointing at the OTel Collector. It returns a shutdown function
// the caller should defer.
//
// Configurable via env:
//
//	OTEL_EXPORTER_OTLP_ENDPOINT  default otel-collector.esp-observability.svc.cluster.local:4317
//	OTEL_SERVICE_NAME            default gateway-go
//
// On exporter init failure it logs and returns a no-op shutdown so the
// service keeps running even if tracing is unavailable.
func setupTracing() func(context.Context) error {
	endpoint := env("OTEL_EXPORTER_OTLP_ENDPOINT", "otel-collector.esp-observability.svc.cluster.local:4317")
	endpoint = stripScheme(endpoint) // gRPC exporter needs host:port
	serviceName := env("OTEL_SERVICE_NAME", "gateway-go")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	exp, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		slog.Error("otel exporter init failed, tracing disabled", "err", err, "endpoint", endpoint)
		return func(context.Context) error { return nil }
	}

	res, rerr := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(serviceName)),
	)
	if rerr != nil {
		slog.Error("otel resource init failed", "err", rerr)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp), // BatchSpanProcessor
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	slog.Info("otel tracing initialized", "endpoint", endpoint, "service", serviceName)
	return tp.Shutdown
}

func stripScheme(s string) string {
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	return s
}
