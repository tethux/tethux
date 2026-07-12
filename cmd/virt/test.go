package virt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"slices"
	"strings"
	"time"

	"github.com/moby/moby/client"
	"github.com/spf13/cobra"

	libvirt "github.com/0xveya/tethux/internal/libtethux/virt"
	"github.com/0xveya/tethux/internal/libtethux/virt/container"
)

var defaultTestImages = []string{
	"public.ecr.aws/docker/library/alpine:3.20",
	"public.ecr.aws/docker/library/busybox:1.36",
}

type testOptions struct {
	provider string
	socket   string
	host     string
	output   string
	images   []string
}

type testEvent struct {
	Schema     string         `json:"schema"`
	Timestamp  time.Time      `json:"timestamp"`
	StartedAt  time.Time      `json:"started_at"`
	FinishedAt time.Time      `json:"finished_at"`
	Host       string         `json:"host,omitempty"`
	Provider   string         `json:"provider"`
	Image      string         `json:"image,omitempty"`
	API        string         `json:"api,omitempty"`
	Operation  string         `json:"operation"`
	Status     string         `json:"status"`
	DurationMS int64          `json:"duration_ms"`
	Node       *libvirt.Node  `json:"node,omitempty"`
	Details    map[string]any `json:"details,omitempty"`
	Error      string         `json:"error,omitempty"`
}

type eventWriter struct {
	format string
	host   string
	out    io.Writer
	enc    *json.Encoder
}

func newEventWriter(format, host string) (*eventWriter, error) {
	return newEventWriterTo(format, host, os.Stdout)
}

func newEventWriterTo(format, host string, out io.Writer) (*eventWriter, error) {
	if format != "json" && format != "text" {
		return nil, fmt.Errorf("unknown output format %q (choose json or text)", format)
	}
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	return &eventWriter{format: format, host: host, out: out, enc: enc}, nil
}

// The value form keeps call sites readable when emitting one-off event literals.
//
//nolint:gocritic // testEvent is bounded and immediately serialized.
func (w *eventWriter) emit(event testEvent) error {
	event.Schema = "tethux.provider-test/v1"
	if event.FinishedAt.IsZero() {
		event.FinishedAt = time.Now().UTC()
	}
	if event.StartedAt.IsZero() {
		event.StartedAt = event.FinishedAt.Add(-time.Duration(event.DurationMS) * time.Millisecond)
	}
	event.Timestamp = event.FinishedAt
	if event.Host == "" {
		event.Host = w.host
	}
	if w.format == "json" {
		return w.enc.Encode(event)
	}
	image := ""
	if event.Image != "" {
		image = " image=" + event.Image
	}
	fmt.Fprintf(w.out, "[%s] %-9s %-9s api=%s%s duration=%dms", event.Provider, event.Status, event.Operation, event.API, image, event.DurationMS)
	if event.Error != "" {
		fmt.Fprintf(w.out, " error=%s", event.Error)
	}
	_, err := fmt.Fprintln(w.out)
	return err
}

func testCmd() *cobra.Command {
	opts := testOptions{}
	c := &cobra.Command{
		Use:   "test",
		Short: "exercise every container provider operation with structured output",
		RunE: func(c *cobra.Command, _ []string) error {
			if len(opts.images) < 2 {
				return fmt.Errorf("provider tests require at least two images")
			}
			if opts.host != "" {
				return runRemoteTest(c.Context(), &opts)
			}
			writer, err := newEventWriter(opts.output, localHostname())
			if err != nil {
				return err
			}
			providers := []string{opts.provider}
			if opts.provider == "all" {
				providers = []string{"docker", "podman", "containerd"}
			}
			for _, provider := range providers {
				if err := runProviderTest(c.Context(), writer, provider, opts.socket, opts.images); err != nil {
					return err
				}
			}
			return nil
		},
	}
	addProviderFlags(c, &opts.provider, &opts.socket)
	c.Flags().StringVar(&opts.host, "host", os.Getenv(testHostEnv), "SSH host for remote provider test, or "+testHostEnv)
	c.Flags().StringVarP(&opts.output, "output", "o", "json", "output format: json or text")
	c.Flags().StringSliceVar(&opts.images, "images", slices.Clone(defaultTestImages), "two or more image references to test")
	return c
}

func runProviderTest(ctx context.Context, writer *eventWriter, provider, socket string, images []string) error {
	started := time.Now().UTC()
	p, err := newProvider(provider, socket)
	if err != nil {
		finished := time.Now().UTC()
		_ = writer.emit(testEvent{Provider: provider, Operation: "connect", Status: "failed", Error: err.Error(), StartedAt: started, FinishedAt: finished, DurationMS: finished.Sub(started).Milliseconds()})
		return err
	}
	finished := time.Now().UTC()
	if err := writer.emit(testEvent{Provider: provider, Operation: "connect", Status: "passed", StartedAt: started, FinishedAt: finished, DurationMS: finished.Sub(started).Milliseconds(), Details: map[string]any{"images": len(images)}}); err != nil {
		return err
	}

	for index, image := range images {
		api := "provider"
		if index%2 == 1 {
			api = "container"
		}
		if err := runImageTest(ctx, writer, p, provider, image, api, index); err != nil {
			return err
		}
	}
	return writer.emit(testEvent{Provider: provider, Operation: "summary", Status: "passed", Details: map[string]any{"images": len(images)}})
}

func runImageTest(ctx context.Context, writer *eventWriter, p container.ContainerProvider, provider, image, api string, index int) (resultErr error) {
	name := fmt.Sprintf("tethux-ci-%s-%d", provider, index)
	call := func(operation string, fn func() error) error {
		started := time.Now().UTC()
		err := fn()
		finished := time.Now().UTC()
		event := testEvent{Provider: provider, Image: image, API: api, Operation: operation, Status: "passed", StartedAt: started, FinishedAt: finished, DurationMS: finished.Sub(started).Milliseconds()}
		if err != nil {
			event.Status = "failed"
			event.Error = err.Error()
		}
		if emitErr := writer.emit(event); emitErr != nil {
			return emitErr
		}
		return err
	}

	_ = p.Delete(ctx, name)
	defer func() {
		if resultErr != nil {
			_ = p.DeleteContainer(context.WithoutCancel(ctx), name, &client.ContainerRemoveOptions{Force: true})
		}
	}()

	if err := call("pull", func() error { return p.Pull(ctx, image, nil) }); err != nil {
		return err
	}

	// Exercise the base Provider.Create path independently; container images
	// with short-lived defaults are valid here because creation is metadata-only.
	baseName := name + "-base"
	_ = p.Delete(ctx, baseName)
	if err := call("create", func() error {
		_, err := p.Create(ctx, &libvirt.NodeConfig{Name: baseName, Image: image})
		return err
	}); err != nil {
		return err
	}
	if err := call("delete", func() error { return p.Delete(ctx, baseName) }); err != nil {
		return err
	}

	var node *container.ContainerNode
	if err := call("create-container", func() error {
		var err error
		node, err = p.CreateContainer(ctx, &container.ContainerConfig{
			NodeConfig: libvirt.NodeConfig{Name: name, Image: image},
			Cmd:        []string{"sh", "-c", "echo tethux-ready; trap 'exit 0' TERM; while :; do sleep 1; done"},
			Env:        []string{"TETHUX_TEST=structured"},
			Labels:     map[string]string{"io.tethux.test": "provider-suite", "io.tethux.api": api},
		})
		return err
	}); err != nil {
		return err
	}

	start := func() error {
		if api == "container" {
			return p.StartContainer(ctx, node.ID, nil)
		}
		return p.Start(ctx, node.ID)
	}
	if err := call("start", start); err != nil {
		return err
	}
	time.Sleep(150 * time.Millisecond)

	if err := call("state-running", func() error {
		state, err := p.State(ctx, node.ID)
		if err == nil && state != libvirt.NodeRunning {
			return fmt.Errorf("expected running, got %s", state)
		}
		return err
	}); err != nil {
		return err
	}
	if err := call("reload", func() error {
		reloaded, err := p.Reload(ctx, node.ID)
		if err == nil && reloaded.ID == "" {
			return errors.New("reload returned an empty ID")
		}
		return err
	}); err != nil {
		return err
	}
	if err := call("list", func() error {
		nodes, err := p.List(ctx)
		if err != nil {
			return err
		}
		for _, listed := range nodes {
			if listed.ID == node.ID {
				return nil
			}
		}
		return fmt.Errorf("created node %q missing from list", node.ID)
	}); err != nil {
		return err
	}
	if err := call("inspect", func() error {
		inspected, err := p.Inspect(ctx, node.ID, nil)
		if err == nil && inspected.ImageName == "" {
			return errors.New("inspect returned an empty image name")
		}
		if err == nil && inspected.PID == 0 {
			return errors.New("inspect returned an empty runtime PID")
		}
		return err
	}); err != nil {
		return err
	}
	if err := call("exec", func() error {
		stdout, stderr, err := p.Exec(ctx, node.ID, []string{"sh", "-c", "printf %s \"$TETHUX_TEST\""}, nil, nil)
		if err != nil {
			return err
		}
		if strings.TrimSpace(string(stdout)) != "structured" {
			return fmt.Errorf("unexpected stdout %q (stderr %q)", stdout, stderr)
		}
		return nil
	}); err != nil {
		return err
	}
	if err := call("logs", func() error {
		reader, err := p.Logs(ctx, node.ID, nil)
		if err != nil {
			return err
		}
		defer reader.Close()
		logs, err := io.ReadAll(reader)
		if err == nil && !strings.Contains(string(logs), "tethux-ready") {
			return fmt.Errorf("expected readiness marker in logs")
		}
		return err
	}); err != nil {
		return err
	}

	suspend := func() error {
		if api == "container" {
			return p.SuspendContainer(ctx, node.ID, nil)
		}
		return p.Suspend(ctx, node.ID)
	}
	if err := call("suspend", suspend); err != nil {
		return err
	}
	if err := call("state-suspended", func() error {
		state, err := p.State(ctx, node.ID)
		if err == nil && state != libvirt.NodeSuspended {
			return fmt.Errorf("expected suspended, got %s", state)
		}
		return err
	}); err != nil {
		return err
	}
	resume := func() error {
		if api == "container" {
			return p.ResumeContainer(ctx, node.ID, nil)
		}
		return p.Resume(ctx, node.ID)
	}
	if err := call("resume", resume); err != nil {
		return err
	}
	restart := func() error {
		if api == "container" {
			return p.RestartContainer(ctx, node.ID, nil)
		}
		return p.Restart(ctx, node.ID)
	}
	if err := call("restart", restart); err != nil {
		return err
	}
	stop := func() error {
		if api == "container" {
			return p.StopContainer(ctx, node.ID, nil)
		}
		return p.Stop(ctx, node.ID)
	}
	if err := call("stop", stop); err != nil {
		return err
	}
	if err := call("state-stopped", func() error {
		state, err := p.State(ctx, node.ID)
		if err == nil && state != libvirt.NodeStopped {
			return fmt.Errorf("expected stopped, got %s", state)
		}
		return err
	}); err != nil {
		return err
	}
	remove := func() error {
		if api == "container" {
			return p.DeleteContainer(ctx, node.ID, &client.ContainerRemoveOptions{Force: true})
		}
		return p.Delete(ctx, node.ID)
	}
	return call("delete-container", remove)
}

func runRemoteTest(ctx context.Context, opts *testOptions) error {
	parts := []string{"sudo", "-n", "env", testHostEnv + "=", "tethux", "virt", "test", "--provider", opts.provider, "--output", opts.output}
	if opts.socket != "" {
		parts = append(parts, "--socket", opts.socket)
	}
	for _, image := range opts.images {
		parts = append(parts, "--images", image)
	}
	ssh := osexec.CommandContext(ctx, "ssh", "-o", "BatchMode=yes", opts.host, shellJoin(parts)) // #nosec G204 -- explicit --host transport.
	ssh.Stdin = os.Stdin
	ssh.Stdout = os.Stdout
	ssh.Stderr = os.Stderr
	return ssh.Run()
}

func localHostname() string {
	host, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return host
}
