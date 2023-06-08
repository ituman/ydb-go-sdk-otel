package ydb

import (
	"context"
	"errors"
	"net/url"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/meta"
	"github.com/ydb-platform/ydb-go-sdk/v3/retry"
)

const (
	errorAttribute = "error"
	tracerID       = "ydb-go-sdk"
	version        = "v" + ydb.Version
)

func logError(s trace.Span, err error, fields ...attribute.KeyValue) {
	s.RecordError(err, trace.WithAttributes(append(fields, attribute.Bool(errorAttribute, true))...))
	s.SetStatus(codes.Error, err.Error())
	m := retry.Check(err)
	s.SetAttributes(
		attribute.Bool(errorAttribute+".delete_session", m.MustDeleteSession()),
		attribute.Bool(errorAttribute+".must_retry", m.MustRetry(false)),
		attribute.Bool(errorAttribute+".must_retry_idempotent", m.MustRetry(true)),
	)
	var ydbErr ydb.Error
	if errors.As(err, &ydbErr) {
		s.SetAttributes(
			attribute.Int(errorAttribute+".ydb.code", int(ydbErr.Code())),
			attribute.String(errorAttribute+".ydb.name", ydbErr.Name()),
		)
	}
}

func finish(s trace.Span, err error, fields ...attribute.KeyValue) {
	if err != nil {
		logError(s, err, fields...)
	} else {
		s.SetAttributes(fields...)
	}
	s.End()
}

//nolint:unparam
func intermediate(s trace.Span, err error, fields ...attribute.KeyValue) {
	if err != nil {
		logError(s, err, fields...)
	} else {
		s.SetAttributes(fields...)
	}
}

type counter struct {
	span    trace.Span
	counter int64
	name    string
}

func startSpanWithCounter(
	tracer trace.Tracer,
	ctx *context.Context,
	operationName string,
	counterName string,
	fields ...attribute.KeyValue,
) (c *counter) {
	fields = append(fields, attribute.String("ydb.driver.sensor", operationName+"_"+counterName))
	return &counter{
		span:    startSpan(tracer, ctx, operationName, fields...),
		counter: 0,
		name:    counterName,
	}
}

func startSpan(
	tracer trace.Tracer,
	ctx *context.Context,
	operationName string,
	fields ...attribute.KeyValue,
) (s trace.Span) {
	fields = append(fields, attribute.String("ydb-go-sdk", version))
	*ctx, s = tracer.Start(
		*ctx,
		operationName,
		trace.WithAttributes(fields...),
	)
	*ctx = meta.WithTraceID(*ctx, s.SpanContext().TraceID().String())
	return s
}

func followSpan(
	tracer trace.Tracer,
	related trace.SpanContext,
	ctx *context.Context,
	operationName string,
	fields ...attribute.KeyValue,
) (s trace.Span) {
	fields = append(fields, attribute.String("ydb-go-sdk", version))
	*ctx, s = tracer.Start(
		trace.ContextWithRemoteSpanContext(*ctx, related),
		operationName,
		trace.WithAttributes(fields...),
	)
	return s
}

func nodeID(sessionID string) string {
	u, err := url.Parse(sessionID)
	if err != nil {
		return ""
	}
	return u.Query().Get("node_id")
}
