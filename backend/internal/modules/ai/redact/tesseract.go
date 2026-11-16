package redact

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"image"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Tesseract runs the tesseract command-line OCR (TSV output). It needs the
// language data of Lang installed (Alpine: tesseract-ocr-data-tur).
type Tesseract struct {
	Path    string
	Lang    string
	Timeout time.Duration
}

// NewTesseract returns the engine when the binary is found on PATH (or at
// path), or an error.
func NewTesseract(path, lang string) (*Tesseract, error) {
	if path == "" {
		path = "tesseract"
	}
	if lang == "" {
		lang = "tur"
	}
	full, err := exec.LookPath(path)
	if err != nil {
		return nil, fmt.Errorf("redact: tesseract not found: %w", err)
	}
	return &Tesseract{Path: full, Lang: lang, Timeout: 60 * time.Second}, nil
}

// Words implements OCR.
func (t *Tesseract) Words(ctx context.Context, png []byte) ([]Word, error) {
	if t.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, t.Timeout)
		defer cancel()
	}
	// Page segmentation 3: automatic, no orientation detection.
	cmd := exec.CommandContext(ctx, t.Path, "stdin", "stdout", "-l", t.Lang, "--psm", "3", "tsv")
	cmd.Stdin = bytes.NewReader(png)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("tesseract: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return ParseTSV(stdout.Bytes())
}

// ParseTSV reads tesseract's TSV output (level 5 rows are words). Lines are
// renumbered from 1 in reading order.
func ParseTSV(data []byte) ([]Word, error) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	lineNo := map[[3]int]int{}
	var out []Word
	header := true
	for sc.Scan() {
		if header {
			header = false
			continue
		}
		f := strings.Split(sc.Text(), "\t")
		if len(f) < 12 || f[0] != "5" {
			continue
		}
		n := make([]int, 10)
		for i := 0; i < 10; i++ {
			v, err := strconv.Atoi(f[i])
			if err != nil {
				return nil, fmt.Errorf("tesseract tsv: column %d: %w", i, err)
			}
			n[i] = v
		}
		text := strings.TrimSpace(f[11])
		if text == "" {
			continue
		}
		key := [3]int{n[2], n[3], n[4]} // block, paragraph, line
		ln, ok := lineNo[key]
		if !ok {
			ln = len(lineNo) + 1
			lineNo[key] = ln
		}
		left, top, w, h := n[6], n[7], n[8], n[9]
		out = append(out, Word{Text: text, Line: ln, Box: image.Rect(left, top, left+w, top+h)})
	}
	return out, sc.Err()
}
