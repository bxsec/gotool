package share

import (
	"github.com/bxsec/gotool/server"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/net/context"

	"go.opentelemetry.io/otel/propagation"
)

type OpenTelemetryKeyType int

const (
	OpenTelemetryKey OpenTelemetryKeyType = iota
)

type metadataSupplier struct {
	metadata map[string]string
}

var _ propagation.TextMapCarrier = &metadataSupplier{}

func (s *metadataSupplier) Get(key string) string {
	return s.metadata[key]
}

func (s *metadataSupplier) Set(key string, value string) {
	s.metadata[key] = value
}

func (s *metadataSupplier) Keys() []string {
	out := make([]string, 0, len(s.metadata))
	for key := range s.metadata {
		out = append(out, key)
	}
	return out
}

func Inject(ctx context.Context, propagators propagation.TextMapPropagator) {
	meta := ctx.Value(ReqMetaDataKey)
	if meta == nil {
		meta = make(map[string]string)
		if rpcxContext, ok := ctx.(server.Context); ok {
			rpcxContext.Value(ReqMetaDataKey)
		}
	}

	propagators.Inject(ctx, &metadataSupplier{
		metadata: meta.(map[string]string),
	})
}

func Extract(ctx context.Context, propagators propagation.TextMapPropagator) trace.SpanContext {
	meta := ctx.Value(ReqMetaDataKey)
	if meta == nil {
		meta = make(map[string]string)
		if rpcxContext, ok := ctx.(server.Context); ok {
			rpcxContext.SetValue(ReqMetaDataKey, meta)
		}
	}

	ctx = propagators.Extract(ctx, &metadataSupplier{
		metadata: meta.(map[string]string),
	})

	return trace.SpanContextFromContext(ctx)
}
