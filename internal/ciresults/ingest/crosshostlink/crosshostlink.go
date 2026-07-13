package crosshostlink

import (
	"encoding/json"
	"fmt"
	"time"
)

type Event struct {
	Schema     string    `json:"schema"`
	Host       string    `json:"host"`
	Provider   Provider  `json:"provider"`
	Name       string    `json:"name"`
	Address    string    `json:"address"`
	Peer       string    `json:"peer"`
	Status     Status    `json:"status"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	DurationMS int64     `json:"duration_ms"`
}

type Provider string

const (
	ProviderDocker     Provider = "docker"
	ProviderPodman     Provider = "podman"
	ProviderContainerd Provider = "containerd"
)

func (p *Provider) UnmarshalJSON(data []byte) error {
	return decodeEnum(data, p, "provider", ProviderDocker, ProviderPodman, ProviderContainerd)
}

type Status string

const (
	StatusReady     Status = "ready"
	StatusPassed    Status = "passed"
	StatusFailed    Status = "failed"
	StatusError     Status = "error"
	StatusCancelled Status = "cancelled"
)

func (s *Status) UnmarshalJSON(data []byte) error {
	return decodeEnum(data, s, "status", StatusReady, StatusPassed, StatusFailed, StatusError, StatusCancelled)
}

func decodeEnum[T ~string](data []byte, target *T, name string, allowed ...T) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("%s must be a string: %w", name, err)
	}
	for _, candidate := range allowed {
		if value == string(candidate) {
			*target = candidate
			return nil
		}
	}
	return fmt.Errorf("unsupported %s %q", name, value)
}
