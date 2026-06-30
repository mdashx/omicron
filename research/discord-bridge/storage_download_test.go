package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/bwmarrin/discordgo"
)

// newDownloadService builds a BridgeService whose downloads land in a temp dir.
func newDownloadService(t *testing.T) (*BridgeService, string) {
	t.Helper()
	dir := t.TempDir()
	return &BridgeService{cfg: Config{DownloadsRoot: dir}}, dir
}

// TestDownloadAttachmentRetriesThenSucceeds verifies a transient 5xx is retried
// and the file is ultimately written intact.
func TestDownloadAttachmentRetriesThenSucceeds(t *testing.T) {
	const body = "hello-attachment-bytes"
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusBadGateway) // first attempt fails transiently
			return
		}
		fmt.Fprint(w, body)
	}))
	defer srv.Close()

	s, dir := newDownloadService(t)
	att := &discordgo.MessageAttachment{ID: "a1", Filename: "note.txt", URL: srv.URL}

	path, err := s.downloadAttachment("chan1", "msg1", att)
	if err != nil {
		t.Fatalf("downloadAttachment: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(got) != body {
		t.Fatalf("content = %q, want %q", got, body)
	}
	if calls < 2 {
		t.Fatalf("expected a retry, got %d call(s)", calls)
	}
	// No leftover temp/partial files in the channel dir.
	assertNoPartials(t, filepath.Join(dir, "chan1"))
}

// TestDownloadAttachmentPermanentFailureLeavesNoFile verifies that when every
// attempt fails, no partial/final file is left at the destination path.
func TestDownloadAttachmentPermanentFailureLeavesNoFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound) // permanent: not retried
	}))
	defer srv.Close()

	s, dir := newDownloadService(t)
	att := &discordgo.MessageAttachment{ID: "a2", Filename: "missing.bin", URL: srv.URL}

	if _, err := s.downloadAttachment("chan2", "msg2", att); err == nil {
		t.Fatal("expected error on permanent failure")
	}
	dest := filepath.Join(dir, "chan2", "msg2_missing.bin")
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("expected no file at %s, stat err = %v", dest, err)
	}
	assertNoPartials(t, filepath.Join(dir, "chan2"))
}

// TestDownloadAttachmentSkipsExisting verifies an already-downloaded file is not
// re-fetched.
func TestDownloadAttachmentSkipsExisting(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		fmt.Fprint(w, "data")
	}))
	defer srv.Close()

	s, _ := newDownloadService(t)
	att := &discordgo.MessageAttachment{ID: "a3", Filename: "dup.txt", URL: srv.URL}

	if _, err := s.downloadAttachment("chan3", "msg3", att); err != nil {
		t.Fatalf("first download: %v", err)
	}
	if _, err := s.downloadAttachment("chan3", "msg3", att); err != nil {
		t.Fatalf("second download: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 HTTP call (second served from disk), got %d", calls)
	}
}

func assertNoPartials(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".part" || len(e.Name()) >= 3 && e.Name()[:3] == ".dl" {
			t.Fatalf("leftover temp file: %s", e.Name())
		}
	}
}
