// Package uploadbody gives the photo and file upload routes their own
// connection deadlines and tells a cut transfer from a bad request (#780).
//
// Uploads come from phones on mobile networks: a 2-4 MB camera photo (five
// of them for a Z report in parts) often takes longer than the server's
// general HTTP_READ_TIMEOUT (10 s) to arrive, and a receipt capture then
// reads the photo with AI before answering, longer than HTTP_WRITE_TIMEOUT
// (30 s). With the general timeouts the body was cut mid-upload and the
// request answered 400, which the mobile queue treats as final, so the
// receipt never reached the office.
package uploadbody

import (
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"time"
)

const (
	// ReadTime bounds reading one upload request's body.
	ReadTime = 3 * time.Minute
	// WriteTime bounds the whole request until the answer is written (body
	// upload plus, for a receipt, the AI read of every part).
	WriteTime = 6 * time.Minute
)

// Extend lifts the connection's read and write deadlines for an upload
// request. Call it before reading the body. A writer that cannot reach the
// connection (a test recorder, a wrapper without Unwrap) keeps the server's
// defaults.
func Extend(w http.ResponseWriter) {
	rc := http.NewResponseController(w)
	now := time.Now()
	_ = rc.SetReadDeadline(now.Add(ReadTime))
	_ = rc.SetWriteDeadline(now.Add(WriteTime))
}

// Status maps a body read error (ParseMultipartForm, FormFile, ReadAll) to
// the answer's status: 408 when the body stopped arriving (timeout, cut
// connection; the client sends it again), 413 when it is over the size
// limit, else 400.
func Status(err error) int {
	var maxErr *http.MaxBytesError
	var netErr net.Error
	switch {
	case errors.As(err, &maxErr):
		return http.StatusRequestEntityTooLarge
	case errors.As(err, &netErr) && netErr.Timeout(),
		errors.Is(err, os.ErrDeadlineExceeded),
		errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, net.ErrClosed):
		return http.StatusRequestTimeout
	default:
		return http.StatusBadRequest
	}
}

// TimeoutCode is the error code of a 408 answer.
const TimeoutCode = "UPLOAD_TIMEOUT"

// TimeoutMessage is the message of a 408 answer.
const TimeoutMessage = "the upload did not arrive in time; send it again"
