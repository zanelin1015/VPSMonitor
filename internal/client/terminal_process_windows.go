//go:build windows

package client

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"sync"
)

type terminalProcess struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	cancel context.CancelFunc
	once   sync.Once
}

func startTerminalProcess(ctx context.Context, opts terminalOptions, onOutput func([]byte), onExit func(int, error)) (runningTerminal, error) {
	shell, args := terminalShellCommand(opts.Shell)
	commandCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(commandCtx, shell, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	process := &terminalProcess{cmd: cmd, stdin: stdin, cancel: cancel}
	go readWindowsTerminalOutput(stdout, onOutput)
	go readWindowsTerminalOutput(stderr, onOutput)
	go func() {
		err := cmd.Wait()
		exitCode := 0
		if err != nil {
			exitCode = -1
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				exitCode = exitErr.ExitCode()
			}
		}
		onExit(exitCode, err)
	}()
	return process, nil
}

func terminalShellCommand(shell string) (string, []string) {
	switch strings.ToLower(strings.TrimSpace(shell)) {
	case "cmd":
		return "cmd.exe", nil
	case "pwsh":
		return "pwsh.exe", []string{"-NoLogo", "-NoProfile"}
	case "powershell":
		fallthrough
	default:
		return "powershell.exe", []string{"-NoLogo", "-NoProfile", "-ExecutionPolicy", "Bypass"}
	}
}

func readWindowsTerminalOutput(reader io.Reader, onOutput func([]byte)) {
	buffered := bufio.NewReader(reader)
	buffer := make([]byte, 4096)
	for {
		n, err := buffered.Read(buffer)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buffer[:n])
			onOutput(chunk)
		}
		if err != nil {
			return
		}
	}
}

func (p *terminalProcess) write(data []byte) error {
	_, err := p.stdin.Write(data)
	return err
}

func (p *terminalProcess) resize(int, int) error {
	return nil
}

func (p *terminalProcess) close() error {
	var err error
	p.once.Do(func() {
		p.cancel()
		_ = p.stdin.Close()
		if p.cmd.Process != nil {
			err = p.cmd.Process.Kill()
		}
	})
	return err
}
