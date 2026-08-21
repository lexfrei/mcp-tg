package telegram

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gotd/td/tg"
)

func docWithName(id int64, name string) *tg.Document {
	doc := &tg.Document{ID: id}
	if name != "" {
		doc.Attributes = []tg.DocumentAttributeClass{
			&tg.DocumentAttributeFilename{FileName: name},
		}
	}

	return doc
}

// The filename arrives from the Telegram server inside the document's
// attributes: it is attacker-controlled input, and it is later joined onto the
// download directory. A traversal component must never survive extraction.
func TestDocumentFileName_SanitizesServerSuppliedNames(t *testing.T) {
	cases := map[string]struct {
		raw  string
		want string
	}{
		"plain":                  {"report.pdf", "report.pdf"},
		"unix traversal":         {"../../escape.jpg", "escape.jpg"},
		"windows traversal":      {"..\\..\\escape.jpg", "escape.jpg"},
		"absolute path":          {"/etc/cron.d/evil", "evil"},
		"parent reference alone": {"..", "document_7"},
		"separator only":         {"///", "document_7"},
		"no filename attribute":  {"", "document_7"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := documentFileName(docWithName(7, tc.raw))
			if got != tc.want {
				t.Errorf("documentFileName(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

// The download target must not be an existing symlink: ToPath would follow it
// and write wherever it points, outside any directory the caller validated.
func TestEnsureNotSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on Windows")
	}

	dir := t.TempDir()

	regular := filepath.Join(dir, "regular.bin")
	writeErr := os.WriteFile(regular, []byte("x"), 0o600)
	if writeErr != nil {
		t.Fatalf("writing file: %v", writeErr)
	}

	link := filepath.Join(dir, "link.bin")
	symErr := os.Symlink(regular, link)
	if symErr != nil {
		t.Fatalf("creating symlink: %v", symErr)
	}

	absentErr := ensureNotSymlink(filepath.Join(dir, "absent.bin"))
	if absentErr != nil {
		t.Errorf("absent path: err = %v, want nil", absentErr)
	}

	regularErr := ensureNotSymlink(regular)
	if regularErr != nil {
		t.Errorf("regular file: err = %v, want nil", regularErr)
	}

	linkErr := ensureNotSymlink(link)
	if linkErr == nil {
		t.Error("symlink: err = nil, want refusal")
	}
}
