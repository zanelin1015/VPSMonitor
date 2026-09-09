package client

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bridge-core/internal/config"
)

type repeatedResponseReader struct {
	remaining int
}

func (r *repeatedResponseReader) Read(p []byte) (int, error) {
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

func TestReadServerAPIResponseRejectsOversizedBody(t *testing.T) {
	_, err := readServerAPIResponse(&repeatedResponseReader{remaining: maxServerAPIResponseBytes + 1})
	if err == nil || !strings.Contains(err.Error(), "response exceeds") {
		t.Fatalf("expected size-limit error, got %v", err)
	}
}

func TestReadServerAPIResponseAcceptsSmallBody(t *testing.T) {
	body, err := readServerAPIResponse(strings.NewReader(`{"agent_id":"node-1"}`))
	if err != nil {
		t.Fatalf("readServerAPIResponse: %v", err)
	}
	if string(body) != `{"agent_id":"node-1"}` {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestServerHTTPClientRejectsCrossOriginRedirect(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://untrusted.example/agent-api", http.StatusFound)
	}))
	defer source.Close()

	app, err := New(config.ClientConfig{ServerURL: source.URL, RequestTimeoutSeconds: 1})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	request, err := http.NewRequest(http.MethodGet, source.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	request.Header.Set("X-Agent-Token", "agent-secret")
	response, err := app.httpClient.Do(request)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusFound {
		t.Fatalf("cross-origin redirect should be returned without following, got %d", response.StatusCode)
	}
}
