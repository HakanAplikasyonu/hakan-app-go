package filetype

import (
	"errors"
	"testing"
)

var (
	pngHead  = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	jpegHead = []byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00")
	pdfHead  = []byte("%PDF-1.7\n%\xe2\xe3\xcf\xd3\n")
	zipHead  = []byte("PK\x03\x04\x14\x00\x00\x00\x08\x00")
	xmlHead  = []byte(`<?xml version="1.0" encoding="UTF-8"?><Invoice>`)
)

func TestInspectAllows(t *testing.T) {
	cases := []struct {
		name, file, declared string
		head                 []byte
		want                 string
	}{
		{"png", "a.png", "image/png", pngHead, "image/png"},
		{"jpeg declared as png keeps sniffed", "a.png", "image/png", jpegHead, "image/jpeg"},
		{"pdf", "a.pdf", "application/pdf", pdfHead, "application/pdf"},
		{"pdf with params", "a.pdf", "application/pdf; charset=binary", pdfHead, "application/pdf"},
		{"zip octet-stream sniffed", "a.zip", "application/octet-stream", zipHead, "application/zip"},
		{"xml declared", "fatura.xml", "application/xml", xmlHead, "application/xml"},
		{"undeclared uses sniffed", "notes.txt", "", []byte("merhaba dünya"), "text/plain"},
		{"svg declared", "logo.svg", "image/svg+xml", []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), "image/svg+xml"},
		{"xlsx keeps declared", "a.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", zipHead, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		{"empty file", "empty.txt", "text/plain", nil, "text/plain"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Inspect(tc.head, tc.file, tc.declared)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("type = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestInspectRejects(t *testing.T) {
	cases := []struct {
		name, file, declared string
		head                 []byte
		want                 error
	}{
		{"pe executable", "a.pdf", "application/pdf", []byte("MZ\x90\x00"), ErrBlocked},
		{"elf", "a.bin", "", []byte("\x7fELF\x02\x01"), ErrBlocked},
		{"mach-o", "a", "", []byte{0xcf, 0xfa, 0xed, 0xfe, 7, 0}, ErrBlocked},
		{"shebang", "a.txt", "text/plain", []byte("#!/usr/bin/env python\n"), ErrBlocked},
		{"exe extension", "Setup.EXE", "", []byte("x"), ErrBlocked},
		{"html extension", "page.htm", "text/plain", []byte("x"), ErrBlocked},
		{"html content", "a.txt", "text/plain", []byte("<!DOCTYPE html><html>"), ErrBlocked},
		{"declared html", "a.dat", "text/html", []byte("x"), ErrBlocked},
		{"declared javascript", "a.dat", "application/javascript", []byte("x"), ErrBlocked},
		{"text claiming png", "a.png", "image/png", []byte("not an image"), ErrMismatch},
		{"zip claiming pdf", "a.pdf", "application/pdf", zipHead, ErrMismatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Inspect(tc.head, tc.file, tc.declared)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}
