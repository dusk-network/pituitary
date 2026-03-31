package openaicompat

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type readCloserWithCloseErr struct {
	io.Reader
	closeErr error
}

func (r readCloserWithCloseErr) Close() error {
	return r.closeErr
}

type stubNetError struct {
	timeout bool
}

func (e stubNetError) Error() string   { return "stub network error" }
func (e stubNetError) Timeout() bool   { return e.timeout }
func (e stubNetError) Temporary() bool { return false }

func TestShouldRetryRetriesTimeoutNetErrors(t *testing.T) {
	t.Parallel()

	if !shouldRetry(stubNetError{timeout: true}, 0) {
		t.Fatal("shouldRetry(timeout net.Error) = false, want true")
	}
}

func TestShouldRetryRetriesKnownConnectionFailures(t *testing.T) {
	t.Parallel()

	if !shouldRetry(errors.New("write tcp: broken pipe"), 0) {
		t.Fatal("shouldRetry(broken pipe) = false, want true")
	}
}

func TestDoWithRetriesReturnsCloseErrorOnSuccessfulResponse(t *testing.T) {
	t.Parallel()

	client := &Client{
		Runtime:    "test-runtime",
		Endpoint:   "https://example.invalid",
		MaxRetries: 0,
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: readCloserWithCloseErr{
						Reader:   strings.NewReader(`{"ok":true}`),
						closeErr: errors.New("close failed"),
					},
					Request: req,
				}, nil
			}),
		},
	}

	_, err := doWithRetries(context.Background(), client, "/v1/test", []byte(`{}`), client.RequestFailureDetails("test"), func(resp *http.Response, body []byte) (string, error) {
		return "ok", nil
	})
	if err == nil {
		t.Fatal("doWithRetries() error = nil, want close error")
	}
	if got := err.Error(); !strings.Contains(got, "close test-runtime response: close failed") {
		t.Fatalf("doWithRetries() error = %q, want close failure", got)
	}
}
