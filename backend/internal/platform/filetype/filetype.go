// Package filetype checks what an uploaded file really is (issue #180): the
// first bytes decide, not the client's Content-Type or file name.
package filetype

import (
	"bytes"
	"errors"
	"mime"
	"net/http"
	"path"
	"strings"
)

// SniffLen is how many leading bytes Inspect needs (as http.DetectContentType).
const SniffLen = 512

var (
	// ErrBlocked: executables, scripts and HTML documents are never stored.
	ErrBlocked = errors.New("filetype: file type is not allowed")
	// ErrMismatch: the declared type says image/PDF but the content is not.
	ErrMismatch = errors.New("filetype: content does not match the declared type")
)

var blockedExt = map[string]bool{
	".exe": true, ".dll": true, ".com": true, ".scr": true, ".msi": true, ".msp": true,
	".bat": true, ".cmd": true, ".ps1": true, ".vbs": true, ".vbe": true, ".wsf": true,
	".js": true, ".mjs": true, ".jse": true, ".hta": true, ".jar": true, ".apk": true,
	".sh": true, ".bash": true, ".elf": true, ".so": true, ".dylib": true, ".app": true,
	".html": true, ".htm": true, ".xhtml": true, ".shtml": true, ".php": true,
}

var blockedMagic = [][]byte{
	[]byte("MZ"),             // PE / DOS executables
	[]byte("\x7fELF"),        // ELF
	{0xfe, 0xed, 0xfa, 0xce}, // Mach-O 32
	{0xfe, 0xed, 0xfa, 0xcf}, // Mach-O 64
	{0xce, 0xfa, 0xed, 0xfe}, // Mach-O 32 LE
	{0xcf, 0xfa, 0xed, 0xfe}, // Mach-O 64 LE
	{0xca, 0xfe, 0xba, 0xbe}, // Mach-O fat / Java class
	[]byte("#!"),             // scripts
}

// Inspect returns the content type to store for a file whose first bytes are
// head (up to SniffLen). It rejects executables, scripts and HTML, and a
// declared image/PDF type the bytes do not back up. Otherwise it keeps the
// declared type when it is specific, falling back to the sniffed one.
func Inspect(head []byte, filename, declared string) (string, error) {
	if blockedExt[strings.ToLower(path.Ext(filename))] {
		return "", ErrBlocked
	}
	for _, m := range blockedMagic {
		if bytes.HasPrefix(head, m) {
			return "", ErrBlocked
		}
	}
	sniffed := http.DetectContentType(head)
	sniffedType, _, _ := mime.ParseMediaType(sniffed)
	if sniffedType == "text/html" {
		return "", ErrBlocked
	}
	declaredType, _, err := mime.ParseMediaType(strings.TrimSpace(declared))
	if err != nil {
		declaredType = ""
	}
	switch declaredType {
	case "text/html", "application/xhtml+xml", "application/x-msdownload",
		"application/x-executable", "application/x-sh", "application/javascript", "text/javascript":
		return "", ErrBlocked
	}
	if len(head) == 0 {
		if declaredType == "" {
			return "application/octet-stream", nil
		}
		return declaredType, nil
	}
	switch {
	case declaredType == "image/svg+xml":
		// SVG sniffs as text/xml or text/plain; it is served as an attachment.
	case strings.HasPrefix(declaredType, "image/"):
		if !strings.HasPrefix(sniffedType, "image/") {
			return "", ErrMismatch
		}
		return sniffedType, nil
	case declaredType == "application/pdf":
		if sniffedType != "application/pdf" {
			return "", ErrMismatch
		}
	}
	if declaredType == "" || declaredType == "application/octet-stream" {
		return sniffedType, nil
	}
	return declaredType, nil
}
