//go:build !windows

package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/creack/pty"
)

type terminalProcess struct {
	cmd     *exec.Cmd
	ptmx    *os.File
	cancel  context.CancelFunc
	closeMu sync.Once
}

func startTerminalProcess(ctx context.Context, opts terminalOptions, onOutput func([]byte), onExit func(int, error)) (runningTerminal, error) {
	shell, args := terminalShellCommand(opts.Shell)
	commandCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(commandCtx, shell, args...)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Rows: uint16(opts.Rows),
		Cols: uint16(opts.Cols),
	})
	if err != nil {
		cancel()
		return nil, err
	}
	process := &terminalProcess{cmd: cmd, ptmx: ptmx, cancel: cancel}
	go process.readOutput(onOutput)
	go process.wait(onExit)
	return process, nil
}

func terminalShellCommand(shell string) (string, []string) {
	shell = strings.ToLower(strings.TrimSpace(shell))
	switch shell {
	case "sh":
		return "sh", nil
	case "pwsh":
		return "pwsh", []string{"-NoLogo"}
	case "powershell":
		return "powershell", []string{"-NoLogo"}
	case "bash":
		fallthrough
	default:
		if commandExists("bash") {
			return "bash", []string{"-l"}
		}
		return "sh", nil
	}
}

func (p *terminalProcess) readOutput(onOutput func([]byte)) {
	buffer := make([]byte, 4096)
	for {
		n, err := p.ptmx.Read(buffer)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buffer[:n])
			onOutput(chunk)
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && !strings.Contains(err.Error(), "input/output error") {
				onOutput([]byte(fmt.Sprintf("\r\n[terminal read error] %v\r\n", err)))
			}
			return
		}
	}
}

func (p *terminalProcess) wait(onExit func(int, error)) {
	err := p.cmd.Wait()
	exitCode := 0
	if err != nil {
		exitCode = -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
	}
	_ = p.ptmx.Close()
	onExit(exitCode, err)
}

func (p *terminalProcess) write(data []byte) error {
	if string(data) == "\r" {
		data = []byte("\n")
	}
	_, err := p.ptmx.Write(data)
	return err
}

func (p *terminalProcess) resize(cols int, rows int) error {
	if cols <= 0 || rows <= 0 {
		return nil
	}
	return pty.Setsize(p.ptmx, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
}

func (p *terminalProcess) close() error {
	var err error
	p.closeMu.Do(func() {
		p.cancel()
		if p.cmd.Process != nil {
			err = p.cmd.Process.Kill()
		}
		_ = p.ptmx.Close()
	})
	return err
}
