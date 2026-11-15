package accountingexport

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"sort"
	"sync"
)

func bytesReader(b []byte) io.Reader { return bytes.NewReader(b) }

// WriteFile writes data through w and returns its File record (SHA-256 is kept
// for "this is the file that went to Luca", docs/02 §4).
func WriteFile(w FileWriter, name, contentType string, data []byte, entries int) (File, error) {
	wc, err := w.Create(name, contentType)
	if err != nil {
		return File{}, err
	}
	if _, err := wc.Write(data); err != nil {
		_ = wc.Close()
		return File{}, err
	}
	if err := wc.Close(); err != nil {
		return File{}, err
	}
	sum := sha256.Sum256(data)
	return File{Name: name, ContentType: contentType, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(data)), Entries: entries}, nil
}

// MemoryWriter keeps produced files in memory (tests, previews).
type MemoryWriter struct {
	mu    sync.Mutex
	files map[string][]byte
}

// NewMemoryWriter returns an empty writer.
func NewMemoryWriter() *MemoryWriter { return &MemoryWriter{files: map[string][]byte{}} }

type memFile struct {
	bytes.Buffer
	name string
	w    *MemoryWriter
}

func (f *memFile) Close() error {
	f.w.mu.Lock()
	defer f.w.mu.Unlock()
	f.w.files[f.name] = append([]byte(nil), f.Bytes()...)
	return nil
}

// Create implements FileWriter.
func (m *MemoryWriter) Create(name, _ string) (io.WriteCloser, error) {
	return &memFile{name: name, w: m}, nil
}

// File returns a written file.
func (m *MemoryWriter) File(name string) ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.files[name]
	return b, ok
}

// Names lists written files, sorted.
func (m *MemoryWriter) Names() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.files))
	for n := range m.files {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
