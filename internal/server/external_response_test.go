package server

import (
	"io"
	"strings"
	"testing"
)

type repeatedExternalResponseReader struct {
	remaining int
}

func (r *repeatedExternalResponseReader) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		return 0, io.EOF
	}
	n := min(len(p), r.remaining)
	for i := range p[:n] {
		p[i] = 'x'
	}
	r.remaining -= n
	return n, nil
}

func TestReadExternalJSONResponseRejectsOversizedBody(t *testing.T) {
	_, err := readExternalJSONResponse(&repeatedExternalResponseReader{remaining: maxExternalJSONResponseBytes + 1})
	if err == nil || !strings.Contains(err.Error(), "response exceeds") {
		t.Fatalf("expected size-limit error, got %v", err)
	}
}
