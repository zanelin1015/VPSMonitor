package client

import (
	"context"
	"testing"

	"bridge-core/internal/model"
)

type fakeRunningTerminal struct {
	writes [][]byte
}

func (f *fakeRunningTerminal) write(data []byte) error {
	copied := append([]byte(nil), data...)
	f.writes = append(f.writes, copied)
	return nil
}

func (f *fakeRunningTerminal) resize(cols int, rows int) error {
	return nil
}

func (f *fakeRunningTerminal) close() error {
	return nil
}

func TestTerminalInputPreservesRawControlBytes(t *testing.T) {
	cases := []struct {
		name string
		data string
	}{
		{name: "newline", data: "\n"},
		{name: "carriage return", data: "\r"},
		{name: "space", data: " "},
		{name: "tab", data: "\t"},
		{name: "command", data: "x-ui\r"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			terminal := &fakeRunningTerminal{}
			manager := newTerminalManager("agent-1", func(model.TerminalMessage) {})
			manager.sessions["session-1"] = terminal

			manager.handleControl(context.Background(), model.AgentControlMessage{
				Type: model.AgentControlTerminalInput,
				Payload: map[string]any{
					"session_id": "session-1",
					"data":       tc.data,
				},
			})

			if len(terminal.writes) != 1 {
				t.Fatalf("expected one write, got %d", len(terminal.writes))
			}
			if got := string(terminal.writes[0]); got != tc.data {
				t.Fatalf("expected raw input %q, got %q", tc.data, got)
			}
		})
	}
}
