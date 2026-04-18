package ui

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestDebugfRespectsDebugLevel(t *testing.T) {
	SetDebug(false)
	output := captureStdout(t, func() {
		Debugf("hidden message")
	})
	if strings.Contains(output, "hidden message") {
		t.Fatalf("expected debug output to be hidden")
	}

	SetDebug(true)
	output = captureStdout(t, func() {
		Debugf("visible message")
	})
	if !strings.Contains(output, "visible message") {
		t.Fatalf("expected debug output when debug is enabled")
	}
	SetDebug(false)
}

func TestPrintSolidTableAddsTrailingBlankLine(t *testing.T) {
	output := captureStdout(t, func() {
		PrintSolidTable([]string{"a"}, [][]string{{"b"}})
	})
	if !strings.HasSuffix(output, "\n\n") {
		t.Fatalf("expected table output to end with a blank line, got %q", output)
	}
}

func TestPrintTableAddsTrailingBlankLine(t *testing.T) {
	output := captureStdout(t, func() {
		PrintTable([]string{"a"}, [][]string{{"b"}})
	})
	if !strings.HasSuffix(output, "\n\n") {
		t.Fatalf("expected table output to end with a blank line, got %q", output)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}

	os.Stdout = writer
	fn()
	writer.Close()
	os.Stdout = originalStdout

	var buffer bytes.Buffer
	if _, err := io.Copy(&buffer, reader); err != nil {
		t.Fatalf("copy captured stdout: %v", err)
	}
	return buffer.String()
}
