// Package pagecursor encodes keyset pagination cursors (issue #182): the
// sort key of the last row of a page, here (date, id). The cursor is opaque
// to clients; it carries no personal data, only an internal row id and a
// date.
package pagecursor

import (
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"
)

// ErrInvalid: the cursor was not produced by Encode.
var ErrInvalid = errors.New("pagecursor: invalid cursor")

const layout = "2006-01-02"

// Encode returns the cursor after the row with this date and id.
func Encode(date time.Time, id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte("d1|" + date.UTC().Format(layout) + "|" + strconv.FormatInt(id, 10)))
}

// Decode parses a cursor from Encode.
func Decode(s string) (time.Time, int64, error) {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return time.Time{}, 0, ErrInvalid
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 3 || parts[0] != "d1" {
		return time.Time{}, 0, ErrInvalid
	}
	d, err := time.Parse(layout, parts[1])
	if err != nil {
		return time.Time{}, 0, ErrInvalid
	}
	id, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || id <= 0 {
		return time.Time{}, 0, ErrInvalid
	}
	return d, id, nil
}
