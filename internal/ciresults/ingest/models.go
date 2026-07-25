package ingest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"

	"github.com/google/uuid"
)

type Variant string

const (
	CrossLaptop Variant = "cross-laptop"
	Laptop78    Variant = "laptop-78"
	Laptop100   Variant = "laptop-100"
	Normal      Variant = "normal"
)

func (v Variant) String() string {
	return string(v)
}

var workflowName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)

func ParseVariant(s string) (Variant, error) {
	if !workflowName.MatchString(s) || s == "." || s == ".." {
		return "", errors.New("invalid workflow name")
	}
	return Variant(s), nil
}

func (v *Variant) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("variant must be a string: %w", err)
	}
	parsed, err := ParseVariant(value)
	if err != nil {
		return fmt.Errorf("invalid variant %q: %w", value, err)
	}
	*v = parsed
	return nil
}

type VariantDetails struct {
	Variant     Variant
	ArchivePath string
}

type ArchiveRef struct {
	Hash     string
	RunID    string // uuidv7
	Variants []VariantDetails
}

type ExtractedRun struct {
	Archive ArchiveRef
	TempDir string
	Variant Variant

	ManifestPath string
	ResultsPath  string
	ConfigsDir   string
	LogsDir      string
	ArtifactsDir string
}

type ExtractedCandidate struct {
	TempDir string
	Runs    []ExtractedRun
}

func (e *ExtractedCandidate) Close() error {
	if e.TempDir == "" {
		return nil
	}

	err := os.RemoveAll(e.TempDir)
	e.TempDir = ""
	return err
}

type IngestionRecord struct {
	Hash        string
	RunID       string
	Variant     Variant
	ArchivePath string
	RunDir      string

	ManifestJSON []byte
	ResultsJSON  []byte

	ConfigsDir   string
	LogsDir      string
	ArtifactsDir string
}

func IsUUIDv7(s string) bool {
	u, err := uuid.Parse(s)
	if err != nil {
		return false
	}

	return u.Version() == 7
}
