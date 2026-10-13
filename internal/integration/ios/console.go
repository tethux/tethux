// Package ios provides bounded native console operations for IOS integration checks.
package ios

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tethux/dynamips-downgrade/console"
	"github.com/tethux/tethux/virt/emulator/dynamips/errs"
)

var promptPattern = regexp.MustCompile(`(?:^|[\r\n])([A-Za-z][A-Za-z0-9_.-]*(?:\([^()\r\n]*\))?[>#])\s*$`)

// Ready waits for boot and disables console pagination.
func Ready(ctx context.Context, ports []int) ([]*console.Connection, error) {
	connections := make([]*console.Connection, 0, len(ports))
	for _, port := range ports {
		address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
		var connection *console.Connection
		for {
			conn, dialErr := console.Dial(ctx, address)
			if dialErr == nil {
				connection = conn
				break
			}
			timer := time.NewTimer(200 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, errors.Join(ctx.Err(), dialErr, Close(connections))
			case <-timer.C:
			}
		}
		connections = append(connections, connection)
		prompt, _, promptErr := readPrompt(ctx, connection, true, "")
		if promptErr != nil {
			return nil, errors.Join(promptErr, Close(connections))
		}
		if strings.HasSuffix(prompt, ">") {
			if _, enableErr := Command(ctx, connection, "enable"); enableErr != nil {
				return nil, errors.Join(enableErr, Close(connections))
			}
		}
		if _, pagingErr := Command(ctx, connection, "terminal length 0"); pagingErr != nil {
			return nil, errors.Join(pagingErr, Close(connections))
		}
	}
	return connections, nil
}

// Close releases every console connection and preserves failures.
func Close(connections []*console.Connection) error {
	failures := make([]error, 0, len(connections))
	for _, connection := range connections {
		failures = append(failures, connection.Close())
	}
	return errors.Join(failures...)
}

// Command waits for an echoed command and its final IOS prompt.
func Command(ctx context.Context, connection *console.Connection, input string) (string, error) {
	if _, writeErr := io.WriteString(connection, input+"\r"); writeErr != nil {
		return "", writeErr
	}
	_, output, readErr := readPrompt(ctx, connection, false, input)
	if readErr != nil {
		return output, readErr
	}
	if strings.Contains(output, "% Invalid") || strings.Contains(output, "% Incomplete") || strings.Contains(output, "% Error") {
		return output, errs.New("IOS command "+input, errs.ErrLifecycle, output)
	}
	return output, nil
}

func readPrompt(ctx context.Context, connection *console.Connection, boot bool, input string) (prompt, transcript string, resultErr error) {
	deadline := time.Now().Add(90 * time.Second)
	var output bytes.Buffer
	buffer := make([]byte, 4096)
	lastNudge := time.Time{}
	for time.Now().Before(deadline) {
		if contextErr := ctx.Err(); contextErr != nil {
			return "", output.String(), contextErr
		}
		if deadlineErr := connection.SetDeadline(time.Now().Add(time.Second)); deadlineErr != nil {
			return "", output.String(), deadlineErr
		}
		count, readErr := connection.Read(buffer)
		output.Write(buffer[:count])
		text := output.String()
		if output.Len() > 2<<20 {
			return "", text, errs.New("read IOS prompt", errs.ErrLifecycle, "console exceeded 2 MiB")
		}
		if match := promptPattern.FindStringSubmatch(text); match != nil && (input == "" || strings.Contains(text, input)) {
			return match[1], text, nil
		}
		if boot && time.Since(lastNudge) > 3*time.Second {
			reply := "\r"
			if strings.Contains(strings.ToLower(text), "initial configuration dialog") {
				reply = "no\r"
			}
			if strings.Contains(strings.ToLower(text), "terminate autoinstall") {
				reply = "yes\r"
			}
			if _, writeErr := io.WriteString(connection, reply); writeErr != nil {
				return "", text, writeErr
			}
			lastNudge = time.Now()
		}
		var timeout net.Error
		if readErr != nil && (!errors.As(readErr, &timeout) || !timeout.Timeout()) {
			return "", text, readErr
		}
	}
	return "", output.String(), errs.New("read IOS prompt", errs.ErrLifecycle, "timed out: "+output.String())
}
