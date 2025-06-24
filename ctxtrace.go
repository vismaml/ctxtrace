package ctxtrace

import (
	"context"
	"net/http"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"github.com/openzipkin/zipkin-go/model"
	"github.com/openzipkin/zipkin-go/propagation/b3"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const (
	headerRequestID  = "x-request-id"
	headerCloudTrace = "x-cloud-trace-context"
	headerB3TraceID  = "x-b3-traceid"
)

// TraceData is a simple struct to hold both the RequestID and the B3 TraceSpan
type TraceData struct {
	RequestID string
	TraceSpan *model.SpanContext
}

type traceCtxMarker struct{}

// UnaryServerInterceptor for propagating client information
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		ctx = extractMetadataToContext(ctx)

		return handler(ctx, req)
	}
}

// StreamServerInterceptor for propagating client information
// only on the first request on the stream
func StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := stream.Context()
		wrapped := grpc_middleware.WrapServerStream(stream)
		wrapped.WrappedContext = extractMetadataToContext(ctx)

		return handler(srv, wrapped)
	}
}

// UnaryClientInterceptor propagates any user information from the context
func UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{},
		cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		newCtx := NewOutgoingContextWithData(ctx)
		return invoker(newCtx, method, req, reply, cc, opts...)
	}
}

// StreamClientInterceptor propagates any user information from the context
func StreamClientInterceptor() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn,
		method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		newCtx := NewOutgoingContextWithData(ctx)
		return streamer(newCtx, desc, cc, method, opts...)
	}
}

// Extract extracts metadata from the context.
func Extract(ctx context.Context) TraceData {
	data, ok := ctx.Value(traceCtxMarker{}).(TraceData)
	if !ok {
		return TraceData{}
	}
	return data
}

// ExtractHTTP extracts metadata from a normal http request
func ExtractHTTP(r *http.Request) (TraceData, error) {
	data := TraceData{}
	if reqID := r.Header.Get(headerRequestID); reqID != "" {
		data.RequestID = reqID
	}
	span, err := b3.ExtractHTTP(r)()
	if err != nil {
		return data, err
	}
	data.TraceSpan = span
	return data, nil
}

// ExtractHTTPToContext extracts metadata from a normal http request and adds it to the context
func ExtractHTTPToContext(ctx context.Context, r *http.Request) context.Context {
	data, _ := ExtractHTTP(r)
	return context.WithValue(ctx, traceCtxMarker{}, data)
}

func addOtelSpanContextToContext(ctx context.Context, traceData TraceData) context.Context {
	traceIDString := traceData.TraceSpan.TraceID.String()
	traceID, err := trace.TraceIDFromHex(traceIDString)
	if err != nil {
		return ctx
	}

	spanIDString := traceData.TraceSpan.ID.String()
	spanID, err := trace.SpanIDFromHex(spanIDString)
	if err != nil {
		return ctx
	}

	traceFlags := trace.TraceFlags(0)
	if *traceData.TraceSpan.Sampled {
		traceFlags = trace.FlagsSampled
	}
	spanContext := trace.NewSpanContext(
		//TODO: add tracestate, remote
		trace.SpanContextConfig{
			TraceID:    traceID,
			SpanID:     spanID,
			TraceFlags: traceFlags,
		},
	)
	if !spanContext.IsValid() {
		return ctx
	}

	return trace.ContextWithRemoteSpanContext(ctx, spanContext)
}

// finds caller information in the gRPC metadata and adds it to the context
func extractMetadataToContext(ctx context.Context) context.Context {
	md, mdOK := metadata.FromIncomingContext(ctx)
	if !mdOK {
		return ctx
	}

	data := TraceData{}

	// Check what headers we have for logging
	hasB3 := len(md[headerB3TraceID]) > 0
	hasCloudTrace := len(md[headerCloudTrace]) > 0

	// Log the header types detected
	if hasB3 && hasCloudTrace {
		zap.L().Info("incoming request has both trace formats",
			zap.Bool("has_b3_headers", hasB3),
			zap.Bool("has_cloud_trace_headers", hasCloudTrace),
		)
	} else if hasB3 {
		zap.L().Info("incoming request has B3 trace headers")
	} else if hasCloudTrace {
		zap.L().Info("incoming request has Cloud Trace headers")
	} else {
		zap.L().Debug("incoming request has no trace headers")
	}

	span, err := b3.ExtractGRPC(&md)()
	if err != nil {
		if hasCloudTrace {
			zap.L().Warn("b3 extract failed but cloud trace headers present",
				zap.Error(err),
				zap.Strings("cloud_trace_headers", md[headerCloudTrace]),
			)
		} else {
			zap.L().Warn("b3 extract failed", zap.Error(err))
		}
	} else {
		data.TraceSpan = span
		ctx = addOtelSpanContextToContext(ctx, data)
		zap.L().Info("b3 trace extraction successful",
			zap.String("trace_id", span.TraceID.String()),
			zap.String("span_id", span.ID.String()),
		)
	}

	if mdValue, ok := md[headerRequestID]; ok && len(mdValue) != 0 {
		data.RequestID = mdValue[0]
		grpc_ctxtags.Extract(ctx).Set("request_id", mdValue[0])
	}

	return context.WithValue(ctx, traceCtxMarker{}, data)
}

// NewOutgoingContextWithData creates a new context with the metadata added
func NewOutgoingContextWithData(ctx context.Context) context.Context {
	md := InjectDataIntoOutMetadata(ctx, Extract(ctx))
	return metadata.NewOutgoingContext(ctx, md)
}

// InjectDataIntoOutMetadata injects the given trace data into metadata fit for an outgoing context
func InjectDataIntoOutMetadata(ctx context.Context, data TraceData) metadata.MD {
	md, mdOK := metadata.FromOutgoingContext(ctx)
	if !mdOK {
		md = metadata.New(nil)
	}
	packCallerMetadata(&md, Extract(ctx))
	return md
}

// packCallerMetadata extracts caller specific values from the context,
// into a MD metadata struct that can be propagated with outgoing gRPC requests
func packCallerMetadata(m *metadata.MD, data TraceData) {
	if m == nil {
		zap.L().Fatal("metadata is nil", zap.Stack("stack"))
	}
	if data.TraceSpan != nil {
		err := b3.InjectGRPC(m)(*data.TraceSpan)
		if err != nil {
			zap.L().Warn("b3 injection failed", zap.Error(err))
		}
	}
	if data.RequestID != "" {
		m.Set(headerRequestID, data.RequestID)
	}
}

// WithValue Creates context with TraceData values
func WithValue(ctx context.Context, traceData TraceData) context.Context {
	return context.WithValue(ctx, traceCtxMarker{}, traceData)
}
