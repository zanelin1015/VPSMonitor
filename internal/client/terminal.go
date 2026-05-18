package client

import (
	"context"
	"fmt"
	"log"
	"sync"

	"bridge-core/internal/model"
)

type terminalSender func(model.TerminalMessage)

type terminalManager struct {
	agentID  string
	send     terminalSender
	mu       sync.Mutex
	sessions map[string]runningTerminal
}

type terminalOptions struct {
	SessionID string
	Shell     string
	Rows      int
	Cols      int
}

type runningTerminal interface {
	write(data []byte) error
	resize(cols int, rows int) error
	close() error
}

func newTerminalManager(agentID string, send terminalSender) *terminalManager {
	return &terminalManager{
		agentID:  agentID,
		send:     send,
		sessions: make(map[string]runningTerminal),
	}
}

func (m *terminalManager) handleControl(ctx context.Context, message model.AgentControlMessage) {
	sessionID := payloadString(message.Payload, "session_id", "")
	if sessionID == "" {
		return
	}
	switch message.Type {
	case model.AgentControlTerminalOpen:
		m.open(ctx, terminalOptions{
			SessionID: sessionID,
			Shell:     payloadString(message.Payload, "shell", ""),
			Rows:      remoteCommandPayloadInt(message.Payload["rows"]),
			Cols:      remoteCommandPayloadInt(message.Payload["cols"]),
		})
	case model.AgentControlTerminalInput:
		data := terminalPayloadRawString(message.Payload, "data")
		if data == "" {
			return
		}
		m.withSession(sessionID, func(session runningTerminal) {
			if err := session.write([]byte(data)); err != nil {
				m.sendError(sessionID, err)
			}
		})
	case model.AgentControlTerminalResize:
		rows := remoteCommandPayloadInt(message.Payload["rows"])
		cols := remoteCommandPayloadInt(message.Payload["cols"])
		m.withSession(sessionID, func(session runningTerminal) {
			if err := session.resize(cols, rows); err != nil {
				m.sendError(sessionID, err)
			}
		})
	case model.AgentControlTerminalClose:
		m.close(sessionID)
	}
}

func terminalPayloadRawString(payload map[string]any, key string) string {
	if value, ok := payload[key].(string); ok {
		return value
	}
	return ""
}

func (m *terminalManager) open(ctx context.Context, opts terminalOptions) {
	m.close(opts.SessionID)
	if opts.Rows <= 0 {
		opts.Rows = 36
	}
	if opts.Cols <= 0 {
		opts.Cols = 120
	}
	sessionCtx, cancel := context.WithCancel(ctx)
	session, err := startTerminalProcess(sessionCtx, opts, func(data []byte) {
		m.send(model.TerminalMessage{
			Type:      model.TerminalMessageOutput,
			SessionID: opts.SessionID,
			AgentID:   m.agentID,
			Data:      string(data),
		})
	}, func(exitCode int, err error) {
		cancel()
		m.mu.Lock()
		delete(m.sessions, opts.SessionID)
		m.mu.Unlock()
		message := model.TerminalMessage{
			Type:      model.TerminalMessageClosed,
			SessionID: opts.SessionID,
			AgentID:   m.agentID,
			ExitCode:  exitCode,
		}
		if err != nil {
			message.Error = err.Error()
		}
		m.send(message)
	})
	if err != nil {
		cancel()
		m.sendError(opts.SessionID, err)
		return
	}
	m.mu.Lock()
	m.sessions[opts.SessionID] = session
	m.mu.Unlock()
	m.send(model.TerminalMessage{
		Type:      model.TerminalMessageOpened,
		SessionID: opts.SessionID,
		AgentID:   m.agentID,
		Shell:     opts.Shell,
		Rows:      opts.Rows,
		Cols:      opts.Cols,
	})
}

func (m *terminalManager) withSession(sessionID string, fn func(runningTerminal)) {
	m.mu.Lock()
	session := m.sessions[sessionID]
	m.mu.Unlock()
	if session == nil {
		m.sendError(sessionID, fmt.Errorf("terminal session not found"))
		return
	}
	fn(session)
}

func (m *terminalManager) close(sessionID string) {
	m.mu.Lock()
	session := m.sessions[sessionID]
	delete(m.sessions, sessionID)
	m.mu.Unlock()
	if session != nil {
		if err := session.close(); err != nil {
			log.Printf("close terminal session %s failed: %v", sessionID, err)
		}
	}
}

func (m *terminalManager) closeAll() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.sessions))
	for sessionID := range m.sessions {
		ids = append(ids, sessionID)
	}
	m.mu.Unlock()
	for _, sessionID := range ids {
		m.close(sessionID)
	}
}

func (m *terminalManager) sendError(sessionID string, err error) {
	if err == nil {
		return
	}
	m.send(model.TerminalMessage{
		Type:      model.TerminalMessageError,
		SessionID: sessionID,
		AgentID:   m.agentID,
		Error:     err.Error(),
	})
}
