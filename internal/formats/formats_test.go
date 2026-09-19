package formats

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAcceptPDFOnlyUsesExtensionAndMIME(t *testing.T) {
	dir := t.TempDir()
	pdf := filepath.Join(dir, "paper.pdf")
	if err := os.WriteFile(pdf, []byte("%PDF-1.4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	named := filepath.Join(dir, "paper")
	if err := os.WriteFile(named, []byte("%PDF-1.4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	txt := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(txt, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !Accept(pdf, false) || !Accept(named, false) {
		t.Fatal("PDFs should queue from .pdf or application/pdf")
	}
	if Accept(txt, false) {
		t.Fatal("text file should not queue without expanded formats")
	}
}

func TestViablePDFRejectsEmptyAndMisnamedFiles(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.pdf")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	misnamed := filepath.Join(dir, "download.pdf")
	if err := os.WriteFile(misnamed, []byte("<!doctype html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	valid := filepath.Join(dir, "paper.pdf")
	if err := os.WriteFile(valid, []byte("%PDF-1.7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if Accept(empty, false) || Accept(misnamed, false) || !Accept(valid, false) {
		t.Fatal("PDF viability filter did not distinguish empty, misnamed, and PDF files")
	}
}

func TestAcceptExpandedUsesTerminationsAndMIME(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name, body string
		want       Kind
	}{
		{"photo.jpg", "\xff\xd8\xff\xe0", KindImage},
		{"photo", "\xff\xd8\xff\xe0JFIF", KindImage},
		{"diagram.png", "\x89PNG\r\n\x1a\n", KindImage},
		{"notes.txt", "plain text", KindText},
		{"readme", "this is a text file without an extension\n", KindText},
		{"table.csv", "a,b\n1,2\n", KindTable},
		{"essay.docx", "PK\x03\x04", KindDocument},
		{"binary.bin", "\x00\x01\x02\x03\x04\x05\x06\x07", KindOther},
	}
	for _, tc := range cases {
		path := filepath.Join(dir, tc.name)
		if err := os.WriteFile(path, []byte(tc.body), 0o644); err != nil {
			t.Fatal(err)
		}
		got := Classify(path)
		if got != tc.want {
			t.Errorf("%s: Classify = %v, want %v", tc.name, got, tc.want)
		}
		if Accept(path, true) != (tc.want != KindOther) {
			t.Errorf("%s: Accept(expanded) = %v, kind %v", tc.name, Accept(path, true), got)
		}
		if tc.want != KindPDF && Accept(path, false) {
			t.Errorf("%s: should not queue in PDF-only mode", tc.name)
		}
	}
}
