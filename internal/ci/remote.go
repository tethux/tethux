package ci

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var safeRemotePath = regexp.MustCompile(`^/[A-Za-z0-9._/-]+$`)

type Remote struct {
	Target   string
	JumpHost string
	SSH      string
	SCP      string
}

func (r Remote) sshBinary() string {
	if r.SSH != "" {
		return r.SSH
	}
	return "ssh"
}

func (r Remote) scpBinary() string {
	if r.SCP != "" {
		return r.SCP
	}
	return "scp"
}

func remoteTransportArgs() []string {
	return []string{
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=yes",
		"-o", "UpdateHostKeys=no",
	}
}

func (r Remote) SSHArgs(remoteArgs ...string) ([]string, error) {
	if strings.TrimSpace(r.Target) == "" || strings.HasPrefix(r.Target, "-") {
		return nil, errors.New("remote target is required")
	}
	args := remoteTransportArgs()
	if r.JumpHost != "" {
		args = append(args, "-J", r.JumpHost)
	}
	args = append(args, r.Target, "--")
	args = append(args, remoteArgs...)
	return args, nil
}

func (r Remote) Run(ctx context.Context, stdout, stderr io.Writer, remoteArgs ...string) error {
	args, err := r.SSHArgs(remoteArgs...)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, r.sshBinary(), args...)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	return cmd.Run()
}

func (r Remote) CopyTo(ctx context.Context, source, destination string, stdout, stderr io.Writer) error {
	if !safeRemotePath.MatchString(destination) || strings.Contains(destination, "..") {
		return fmt.Errorf("unsafe remote destination %q", destination)
	}
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	args := append([]string{"-q"}, remoteTransportArgs()...)
	if info.IsDir() {
		args = append(args, "-r")
	}
	if r.JumpHost != "" {
		args = append(args, "-o", "ProxyJump="+r.JumpHost)
	}
	args = append(args, filepath.Clean(source), r.Target+":"+destination)
	cmd := exec.CommandContext(ctx, r.scpBinary(), args...)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	return cmd.Run()
}

func (r Remote) CopyFrom(ctx context.Context, source, destination string, stdout, stderr io.Writer) error {
	if !safeRemotePath.MatchString(source) || strings.Contains(source, "..") {
		return fmt.Errorf("unsafe remote source %q", source)
	}
	if strings.TrimSpace(r.Target) == "" || strings.HasPrefix(r.Target, "-") {
		return errors.New("remote target is required")
	}
	if err := os.MkdirAll(destination, 0o750); err != nil {
		return err
	}
	args := append([]string{"-q", "-r"}, remoteTransportArgs()...)
	if r.JumpHost != "" {
		args = append(args, "-o", "ProxyJump="+r.JumpHost)
	}
	args = append(args, r.Target+":"+source, filepath.Clean(destination))
	cmd := exec.CommandContext(ctx, r.scpBinary(), args...)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	return cmd.Run()
}
