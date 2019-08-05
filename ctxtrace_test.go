package ctxtrace

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/openzipkin/zipkin-go/propagation/b3"
	"github.com/stretchr/testify/assert"
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
	if assert.NotNil(t, outGoingMD) {
		if assert.Contains(t, outGoingMD, headerRequestID) {
			if assert.Len(t, outGoingMD[headerRequestID], 1) {
				assert.Equal(t, dummyRequestID, outGoingMD[headerRequestID][0])
			}
		}
	}
}

func TestExtractHTTP(t *testing.T) {
	r := httptest.NewRequest("GET", "/foo", nil)
	r.Header.Set(headerRequestID, dummyRequestID)
	r.Header.Set(b3.ParentSpanID, strings.Repeat("0", 15)+"1")
	r.Header.Set(b3.Sampled, "0")
	r.Header.Set(b3.SpanID, strings.Repeat("0", 16))
	r.Header.Set(b3.TraceID, strings.Repeat("0", 32))

	data, err := ExtractHTTP(r)
	assert.Nil(t, err)
	assert.NotNil(t, data.TraceSpan)
	assert.Equal(t, dummyRequestID, data.RequestID)
}
