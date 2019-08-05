package ctxtrace

import (
	"context"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"github.com/openzipkin/zipkin-go/model"
	"github.com/openzipkin/zipkin-go/propagation/b3"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const (
	headerRequestID = "x-request-id"
)

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
		newCtx := newOutgoingContextWithData(ctx)
		return invoker(newCtx, method, req, reply, cc, opts...)
	}
}

// StreamClientInterceptor propagates any user information from the context
func StreamClientInterceptor() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn,
		method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		newCtx := newOutgoingContextWithData(ctx)
		return streamer(newCtx, desc, cc, method, opts...)
	}
}

// Extract extracts metadate from the context.
func Extract(ctx context.Context) *TraceData {
	data, ok := ctx.Value(traceCtxMarker{}).(TraceData)
	if !ok {
		return &TraceData{}
	}
	return &data
}

// finds caller information in the gRPC metadata and adds it to the context
func extractMetadataToContext(ctx context.Context) context.Context {
	md, mdOK := metadata.FromIncomingContext(ctx)
	if !mdOK {
		return ctx
	}
	data := TraceData{}
	span, err := b3.ExtractGRPC(&md)()
	if err != nil {
		zap.L().Warn("b3 extract failed", zap.Error(err))
	} else {
		data.TraceSpan = span
	}

	if mdValue, ok := md[headerRequestID]; ok && len(mdValue) != 0 {
		data.RequestID = mdValue[0]
		grpc_ctxtags.Extract(ctx).Set("requestid", mdValue[0])
	}

	return context.WithValue(ctx, traceCtxMarker{}, data)
}

// newOutgoingContextWithData creates a new context with the metadata added
func newOutgoingContextWithData(ctx context.Context) context.Context {
	md, mdOK := metadata.FromOutgoingContext(ctx)
	if !mdOK {
		md = metadata.New(nil)
	}
	packCallerMetadata(ctx, &md)
	return metadata.NewOutgoingContext(ctx, md)
}

// packCallerMetadata extracts caller specific values from the context,
// into a MD metadata struct that can be propagated with outgoing gRPC requests
func packCallerMetadata(ctx context.Context, m *metadata.MD) {
	if m == nil {
		zap.L().Fatal("metadata is nil", zap.Stack("stack"))
	}
	data := Extract(ctx)
	if data.TraceSpan != nil {
		err := b3.InjectGRPC(m)(*data.TraceSpan)
		if err != nil {
			zap.L().Warn("b3 injection failed", zap.Error(err))
		}
	}
	if data.RequestID != "" {
		(*m)[headerRequestID] = []string{data.RequestID}
	}
}
