package ctxtrace

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/metadata"
)

const (
	dummyRequestID = "Foo"
)

func TestPackMetadata(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()
	ctx = context.WithValue(ctx, traceCtxMarker{}, TraceData{
		RequestID: dummyRequestID,
	})

	ctx = newOutgoingContextWithData(ctx)
	outGoingMD, _ := metadata.FromOutgoingContext(ctx)
	if outGoingMD[headerRequestID][0] != dummyRequestID {
		t.Errorf("Unexpected request id: %s", outGoingMD[headerRequestID])
	}
}
