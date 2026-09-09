package panels

import (
	"io"
	"strings"
	"testing"
)

type repeatedByteReader struct {
	remaining int
}

func (r *repeatedByteReader) Read(p []byte) (int, error) {
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

func TestReadPanelResponse(t *testing.T) {
	body, err := readPanelResponse(strings.NewReader(`{"success":true}`))
	if err != nil {
		t.Fatalf("readPanelResponse: %v", err)
	}
	if string(body) != `{"success":true}` {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestReadPanelResponseRejectsOversizedBody(t *testing.T) {
	_, err := readPanelResponse(&repeatedByteReader{remaining: maxPanelResponseBytes + 1})
	if err == nil || !strings.Contains(err.Error(), "response exceeds") {
		t.Fatalf("expected size-limit error, got %v", err)
	}
}
