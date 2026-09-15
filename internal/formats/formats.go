// Package formats decides which files belong in the triage queue.
package formats

import (
	"bytes"
	"context"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Kind is the queue family for a file.
type Kind int

const (
	KindOther Kind = iota
	KindPDF
	KindImage
	KindDocument
	KindTable
	KindText
)

var extKind = map[string]Kind{
	".pdf": KindPDF,

	".jpg": KindImage, ".jpeg": KindImage, ".jpe": KindImage, ".jfif": KindImage,
	".png": KindImage, ".gif": KindImage, ".webp": KindImage, ".bmp": KindImage,
	".tif": KindImage, ".tiff": KindImage, ".svg": KindImage, ".ico": KindImage,
	".heic": KindImage, ".heif": KindImage, ".avif": KindImage, ".jxl": KindImage,

	".doc": KindDocument, ".docx": KindDocument, ".dot": KindDocument, ".odt": KindDocument,
	".rtf": KindDocument, ".epub": KindDocument, ".odp": KindDocument,
	".ppt": KindDocument, ".pptx": KindDocument, ".pages": KindDocument,
	".html": KindDocument, ".htm": KindDocument, ".xhtml": KindDocument,

	".csv": KindTable, ".tsv": KindTable, ".tab": KindTable,
	".xls": KindTable, ".xlsx": KindTable, ".xlsm": KindTable, ".ods": KindTable,

	".txt": KindText, ".text": KindText, ".md": KindText, ".markdown": KindText,
	".rst": KindText, ".log": KindText, ".json": KindText, ".xml": KindText,
	".yaml": KindText, ".yml": KindText, ".toml": KindText, ".ini": KindText,
	".cfg": KindText, ".conf": KindText, ".tex": KindText, ".css": KindText,
	".asc": KindText, ".nfo": KindText, ".org": KindText, ".adoc": KindText,
}

// Accept reports whether path should be queued. PDFs always qualify (extension
// or MIME). Other images, documents, tables, and text files qualify only when
// expanded is true, matching either a known termination or a sniffed MIME type.
func Accept(path string, expanded bool) bool {
	k := Classify(path)
	if k == KindPDF {
		return true
	}
	return expanded && k != KindOther
}

// Classify identifies the file from its extension first, then MIME
// (TypeByExtension, a content sniff, and xdg-mime when the sniff is inconclusive).
func Classify(path string) Kind {
	ext := strings.ToLower(filepath.Ext(path))
	if k, ok := extKind[ext]; ok {
		return k
	}
	for _, raw := range mimeCandidates(path, ext) {
		if k := kindFromMIME(raw); k != KindOther {
			return k
		}
	}
	return KindOther
}

func mimeCandidates(path, ext string) []string {
	var out []string
	if ext != "" {
		if t := mime.TypeByExtension(ext); t != "" {
			out = append(out, t)
		}
	}
	sniffed := sniffContent(path)
	if sniffed != "" {
		out = append(out, sniffed)
	}
	switch sniffed {
	case "", "application/octet-stream", "application/zip", "application/x-ole-storage":
		if t := xdgMIME(path); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func sniffContent(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if n == 0 && err != nil {
		return ""
	}
	return http.DetectContentType(buf[:n])
}

func xdgMIME(path string) string {
	if _, err := exec.LookPath("xdg-mime"); err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "xdg-mime", "query", "filetype", path).Output()
	if err != nil {
		return ""
	}
	return string(bytes.TrimSpace(out))
}

func kindFromMIME(raw string) Kind {
	media, _, err := mime.ParseMediaType(raw)
	if err != nil {
		media = strings.TrimSpace(strings.Split(raw, ";")[0])
	}
	media = strings.ToLower(media)
	switch {
	case media == "application/pdf":
		return KindPDF
	case strings.HasPrefix(media, "image/"):
		return KindImage
	case media == "text/csv", media == "text/tab-separated-values", media == "application/csv",
		strings.Contains(media, "spreadsheet"),
		media == "application/vnd.ms-excel":
		return KindTable
	case media == "text/html", media == "application/xhtml+xml",
		media == "application/rtf", media == "text/rtf",
		media == "application/msword", media == "application/epub+zip",
		strings.Contains(media, "wordprocessingml"),
		strings.Contains(media, "presentationml"),
		media == "application/vnd.ms-powerpoint",
		strings.Contains(media, "opendocument.text"),
		strings.Contains(media, "opendocument.presentation"):
		return KindDocument
	case strings.HasPrefix(media, "text/"),
		media == "application/json", media == "application/xml",
		media == "application/toml", media == "application/yaml":
		return KindText
	default:
		return KindOther
	}
}
