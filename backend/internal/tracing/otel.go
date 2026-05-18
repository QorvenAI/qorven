// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tracing

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

var tracer trace.Tracer

// Init sets up OTel tracing with stdout exporter (swap for OTLP in production).
func Init(serviceName, version string) (func(context.Context) error, error) {
	exp, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(serviceName),
			semconv.ServiceVersionKey.String(version),
		)),
	)
	otel.SetTracerProvider(tp)
	tracer = tp.Tracer(serviceName)
	slog.Info("otel tracing initialized", "service", serviceName)
	return tp.Shutdown, nil
}

// Tracer returns the global tracer.
func Tracer() trace.Tracer { return tracer }

// StartAgentSpan starts a span for an agent run.
func StartAgentSpan(ctx context.Context, agentID, model string) (context.Context, trace.Span) {
	return tracer.Start(ctx, "agent.run",
		trace.WithAttributes(
			attribute.String("agent.id", agentID),
			attribute.String("agent.model", model),
		),
	)
}

// StartToolSpan starts a child span for a tool call.
func StartToolSpan(ctx context.Context, toolName string) (context.Context, trace.Span) {
	return tracer.Start(ctx, "tool."+toolName,
		trace.WithAttributes(attribute.String("tool.name", toolName)),
	)
}

// StartLLMSpan starts a child span for an LLM call.
func StartLLMSpan(ctx context.Context, model string) (context.Context, trace.Span) {
	return tracer.Start(ctx, "llm.chat",
		trace.WithAttributes(attribute.String("llm.model", model)),
	)
}

// SetTokens records token usage on a span.
func SetTokens(span trace.Span, input, output int) {
	span.SetAttributes(
		attribute.Int("llm.input_tokens", input),
		attribute.Int("llm.output_tokens", output),
	)
}
