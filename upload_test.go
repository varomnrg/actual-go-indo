package main

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleUploadSubmit(t *testing.T) {
	// Setup test DB and config
	cfg = Config{
		Addr:       "127.0.0.1:8080",
		SQLitePath: ":memory:",
		UploadDir:  t.TempDir(),
	}

	var err error
	db, err = openDB(cfg.SQLitePath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := seedCategories(db); err != nil {
		t.Fatalf("seed categories: %v", err)
	}

	// Build multipart form with a fake PDF
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	writer.WriteField("bank", "bca")
	part, _ := writer.CreateFormFile("file", "statement.pdf")
	part.Write([]byte("fake pdf content"))
	writer.Close()

	req := httptest.NewRequest("POST", "/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()

	handleUploadSubmit(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected redirect (303), got %d: %s", rec.Code, rec.Body.String())
	}

	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/review/") {
		t.Errorf("expected redirect to /review/..., got %q", loc)
	}
}

func TestHandleUploadSubmitMissingFile(t *testing.T) {
	cfg = Config{
		Addr:       "127.0.0.1:8080",
		SQLitePath: ":memory:",
		UploadDir:  t.TempDir(),
	}

	var err error
	db, err = openDB(cfg.SQLitePath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	writer.WriteField("bank", "bca")
	// Intentionally NOT adding a file
	writer.Close()

	req := httptest.NewRequest("POST", "/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()

	handleUploadSubmit(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing file, got %d", rec.Code)
	}
}

// TestUploadTemplateStructure is a regression test for issue 12.
// The bug was that .file-drop input[type=file] had display:none in CSS,
// and the input was nested inside a label with for="file",
// preventing the file from being submitted in some browsers.
func TestUploadTemplateStructure(t *testing.T) {
	tmpl = template.Must(template.New("").Funcs(tmplFuncs).ParseFS(templateFS, "templates/*.html"))

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "uploadPage", nil); err != nil {
		t.Fatalf("render uploadPage: %v", err)
	}

	html := buf.String()

	// The file input must exist
	if !strings.Contains(html, `<input type="file"`) {
		t.Fatal("upload template missing file input")
	}

	// The file input must NOT be nested inside a label that has for="file"
	// because that creates a double-label association that can break clicks.
	// Find the file input and verify its immediately enclosing tag is NOT <label.
	fileInputIdx := strings.Index(html, `<input type="file"`)
	if fileInputIdx == -1 {
		t.Fatal("file input not found")
	}

	// Look backwards for the immediately preceding opening tag before the input.
	// We scan backwards for '<' to find tags, ignoring the text label on the line above.
	beforeInput := html[:fileInputIdx]
	lastOpenBracket := strings.LastIndex(beforeInput, "<")
	if lastOpenBracket != -1 {
		// Find the end of this tag (space or >)
		tagStart := beforeInput[lastOpenBracket:]
		// Check if it's a label opening tag
		if strings.HasPrefix(tagStart, "<label") || strings.HasPrefix(tagStart, "<LABEL") {
			t.Error("file input is nested inside a <label> — this breaks browser form submission")
		}
	}

	fmt.Fprintf(io.Discard, "html len: %d\n", len(html))
}
