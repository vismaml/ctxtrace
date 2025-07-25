package ctxtrace

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

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

	ctx = NewOutgoingContextWithData(ctx)
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

	data, err := ExtractHTTP(r)
	assert.Nil(t, err)
	assert.Equal(t, dummyRequestID, data.RequestID)
}

func TestExtractHTTPToContext(t *testing.T) {
	r := httptest.NewRequest("GET", "/foo", nil)
	r.Header.Set(headerRequestID, dummyRequestID)

	ctx := context.Background()
	ctx = ExtractHTTPToContext(ctx, r)

	extractedData := Extract(ctx)
	assert.Equal(t, dummyRequestID, extractedData.RequestID)
}

func TestExtractHTTPToContextAndPropagate(t *testing.T) {
	r := httptest.NewRequest("GET", "/foo", nil)
	r.Header.Set(headerRequestID, dummyRequestID)

	ctx := context.Background()
	ctx = ExtractHTTPToContext(ctx, r)

	ctx = NewOutgoingContextWithData(ctx)
	outGoingMD, _ := metadata.FromOutgoingContext(ctx)
	if assert.NotNil(t, outGoingMD) {
		if assert.Contains(t, outGoingMD, headerRequestID) {
			if assert.Len(t, outGoingMD[headerRequestID], 1) {
				assert.Equal(t, dummyRequestID, outGoingMD[headerRequestID][0])
			}
		}
	}
}

func TestExtractHTTPEmptyRequestID(t *testing.T) {
	r := httptest.NewRequest("GET", "/foo", nil)
	// No request ID header set

	data, err := ExtractHTTP(r)
	assert.Nil(t, err)
	assert.Empty(t, data.RequestID)
}

func TestWithValue(t *testing.T) {
	ctx := context.Background()
	traceData := TraceData{RequestID: dummyRequestID}

	ctx = WithValue(ctx, traceData)
	extractedData := Extract(ctx)

	assert.Equal(t, dummyRequestID, extractedData.RequestID)
}
