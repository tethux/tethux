package ingest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/google/uuid"
)

type Variant int

const (
	CrossLaptop Variant = iota
	Laptop78
	Laptop100
	Normal
)

func (v Variant) String() string {
	switch v {
	case CrossLaptop:
		return "cross-laptop"
	case Laptop78:
		return "laptop-78"
	case Laptop100:
		return "laptop-100"
	case Normal:
		return "normal"
	default:
		return "unknown"
	}
}

func ParseVariant(s string) (Variant, error) {
	switch s {
	case "cross-laptop":
		return CrossLaptop, nil
	case "laptop-78":
		return Laptop78, nil
	case "laptop-100":
		return Laptop100, nil
	case "normal":
		return Normal, nil
	default:
		return 0, errors.New("unknown variant")
	}
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
