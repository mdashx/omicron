package harness

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestReadJSONLLineStripsLFAndCRLF(t *testing.T) {
	reader := bufio.NewReader(bytes.NewBufferString("{\"type\":\"x\"}\r\n"))
	line, err := readJSONLLine(reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := string(line), "{\"type\":\"x\"}"; got != want {
		t.Fatalf("unexpected line: got %q want %q", got, want)
	}
}

func TestReadJSONLLineReturnsFinalPartialLineAtEOF(t *testing.T) {
	reader := bufio.NewReader(bytes.NewBufferString("{\"type\":\"x\"}"))
	line, err := readJSONLLine(reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := string(line), "{\"type\":\"x\"}"; got != want {
		t.Fatalf("unexpected line: got %q want %q", got, want)
	}
	_, err = readJSONLLine(reader)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF, got %v", err)
	}
}
