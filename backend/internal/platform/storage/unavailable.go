package storage

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/smithy-go"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/metrics"
)

// ErrUnavailable: the object store refused a write for a server-side reason
// (full, no writable volume, overloaded), issue #247. errors.Is matches it;
// the HTTP layer answers 503 STORAGE_UNAVAILABLE.
var ErrUnavailable = errors.New("storage: object store unavailable")

// Write failure reasons (the storage_write_failures_total label).
const (
	ReasonNoSpace = "no_space"
	ReasonServer  = "server"
	ReasonOther   = "other"
)

// UnavailableError wraps a write failure the client cannot fix by changing
// its request.
type UnavailableError struct {
	Reason string
	Err    error
}

func (e *UnavailableError) Error() string { return "storage: object store unavailable (" + e.Reason + "): " + e.Err.Error() }
func (e *UnavailableError) Unwrap() []error { return []error{ErrUnavailable, e.Err} }

// UnavailableCode implements response.Unavailable.
func (e *UnavailableError) UnavailableCode() string { return "STORAGE_UNAVAILABLE" }

// UnavailableMessage implements response.Unavailable.
func (e *UnavailableError) UnavailableMessage() string {
	return "File storage is temporarily unavailable; please try again later"
}

// noSpaceMarkers are what SeaweedFS (and S3) say when no volume can take the
// object: "No writable volumes and no free volumes left", InsufficientStorage.
var noSpaceMarkers = []string{"no writable volumes", "no free volumes", "insufficientstorage", "insufficient storage", "no space left"}

// classifyWrite returns the failure reason of a write error: "" when the
// error is the client's (bad request, auth, cancelled), else no_space /
// server.
func classifyWrite(err error) string {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return ""
	}
	msg := strings.ToLower(err.Error())
	for _, m := range noSpaceMarkers {
		if strings.Contains(msg, m) {
			return ReasonNoSpace
		}
	}
	var re *awshttp.ResponseError
	if errors.As(err, &re) {
		switch code := re.HTTPStatusCode(); {
		case code == http.StatusInsufficientStorage:
			return ReasonNoSpace
		case code >= 500:
			return ReasonServer
		default:
			return ""
		}
	}
	var api smithy.APIError
	if errors.As(err, &api) {
		switch api.ErrorCode() {
		case "InternalError", "ServiceUnavailable", "SlowDown", "RequestTimeout":
			return ReasonServer
		}
	}
	return ""
}

// writeError wraps a failed write: an UnavailableError when the store is at
// fault (counted in storage_write_failures_total), else the plain error.
func writeError(op string, err error) error {
	reason := classifyWrite(err)
	if reason == "" {
		return fmt.Errorf("storage: %s: %w", op, err)
	}
	metrics.Default.ObserveStorageWriteFailure(reason)
	return &UnavailableError{Reason: reason, Err: fmt.Errorf("%s: %w", op, err)}
}
