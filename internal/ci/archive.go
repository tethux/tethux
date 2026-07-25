package ci

import (
	"archive/tar"
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/0xveya/tethux/internal/ciresults/ingest/archiveformat"
	"github.com/google/uuid"
	"github.com/klauspost/compress/zstd"
)

type ArchiveOptions struct {
	Root       string
	Repository string
	Workflow   string
	DeviceID   string
	Runtime    string
	Revision   string
	RunID      string
	StartedAt  time.Time
	FinishedAt time.Time
	CommandErr error
}

type ArchiveWriter struct {
	Options ArchiveOptions
	Stage   string
}

func NewArchiveWriter(options ArchiveOptions) (*ArchiveWriter, error) {
	if options.Workflow == "" {
		return nil, errors.New("archive workflow is required")
	}
	if options.Root == "" {
		options.Root = filepath.Join("results", "archive")
	}
	if options.Repository == "" {
		root, err := RepositoryRoot()
		if err != nil {
			return nil, err
		}
		options.Repository = root
	}
	if options.Revision == "" {
		revision, err := gitOutput(options.Repository, "rev-parse", "HEAD")
		if err != nil {
			return nil, err
		}
		options.Revision = revision
	}
	if options.RunID == "" {
		id, err := uuid.NewV7()
		if err != nil {
			return nil, fmt.Errorf("create UUIDv7: %w", err)
		}
		options.RunID = id.String()
	}
	stage := filepath.Join(options.Root, options.Revision, options.Workflow, "."+options.RunID+".partial")
	for _, directory := range []string{"logs", "configs", "artifacts"} {
		if err := os.MkdirAll(filepath.Join(stage, directory), 0o750); err != nil {
			return nil, err
		}
	}
	return &ArchiveWriter{Options: options, Stage: stage}, nil
}

// OpenArchiveWriter resumes a previously prepared partial archive directory.
// It is intended for provider integrations that execute and finalize in separate
// processes.
func OpenArchiveWriter(stage string, options ArchiveOptions) (*ArchiveWriter, error) {
	info, err := os.Stat(stage)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || !strings.HasSuffix(filepath.Base(stage), ".partial") {
		return nil, errors.New("archive stage must be an existing .partial directory")
	}
	if options.Workflow == "" || options.Revision == "" || options.RunID == "" {
		return nil, errors.New("workflow, revision, and run ID are required to resume an archive")
	}
	for _, directory := range []string{"logs", "configs", "artifacts"} {
		if err := os.MkdirAll(filepath.Join(stage, directory), 0o750); err != nil {
			return nil, err
		}
	}
	return &ArchiveWriter{Options: options, Stage: filepath.Clean(stage)}, nil
}

func (w *ArchiveWriter) ArtifactDir() string { return filepath.Join(w.Stage, "artifacts") }
func (w *ArchiveWriter) LogDir() string      { return filepath.Join(w.Stage, "logs") }
func (w *ArchiveWriter) ConfigDir() string   { return filepath.Join(w.Stage, "configs") }

func (w *ArchiveWriter) Finalize(ctx context.Context) (string, error) {
	if w.Options.StartedAt.IsZero() {
		w.Options.StartedAt = time.Now().UTC()
	}
	if w.Options.FinishedAt.IsZero() {
		w.Options.FinishedAt = time.Now().UTC()
	}
	results, err := w.collectResults(ctx)
	if err != nil {
		return "", err
	}
	resultsPath := filepath.Join(w.Stage, "results.json")
	if err := writeJSONFile(resultsPath, results); err != nil {
		return "", err
	}
	manifest, err := w.buildManifest(results)
	if err != nil {
		return "", err
	}
	if err := writeJSONFile(filepath.Join(w.Stage, "manifest.json"), manifest); err != nil {
		return "", err
	}
	workflowDir := filepath.Dir(w.Stage)
	partial := filepath.Join(workflowDir, w.Options.RunID+".tar.zst.partial")
	final := strings.TrimSuffix(partial, ".partial")
	if err := writeTarZstd(partial, w.Stage); err != nil {
		return "", err
	}
	if err := os.Rename(partial, final); err != nil {
		return "", err
	}
	if err := writeArchiveDone(final); err != nil {
		return "", err
	}
	if err := os.RemoveAll(w.Stage); err != nil {
		return "", err
	}
	return final, nil
}

func writeArchiveDone(archivePath string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	marker := archivePath + ".done"
	partial := marker + ".partial"
	if err := os.WriteFile(partial, []byte(fmt.Sprintf("%x\n", hash.Sum(nil))), 0o600); err != nil {
		return err
	}
	return os.Rename(partial, marker)
}

func (w *ArchiveWriter) collectResults(ctx context.Context) (archiveformat.ResultsDocument, error) {
	document := archiveformat.ResultsDocument{SchemaVersion: 1, RunID: w.Options.RunID}
	err := filepath.WalkDir(w.ArtifactDir(), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			return walkErr
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 64<<10), 16<<20)
		for scanner.Scan() {
			if err := ctx.Err(); err != nil {
				return err
			}
			if result, ok := normalizeEvent(scanner.Bytes(), path, w.Stage); ok {
				document.Tests = append(document.Tests, result)
			}
		}
		return scanner.Err()
	})
	if err != nil {
		return document, err
	}
	if w.Options.CommandErr != nil && len(document.Tests) == 0 {
		message := w.Options.CommandErr.Error()
		document.Tests = append(document.Tests, archiveformat.TestResult{
			TestID: "ci/workflow/command", Name: "workflow command", Suite: "ci",
			Status: archiveformat.TestStatusError, Attempt: 1, Message: &message,
			Timing: archiveformat.ResultTiming{DurationMS: w.Options.FinishedAt.Sub(w.Options.StartedAt).Milliseconds()},
		})
	}
	return document, nil
}

type goTestEvent struct {
	Time    string  `json:"Time"`
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Elapsed float64 `json:"Elapsed"`
	Output  string  `json:"Output"`
}

func normalizeEvent(line []byte, sourcePath, stage string) (archiveformat.TestResult, bool) {
	var goEvent goTestEvent
	if json.Unmarshal(line, &goEvent) == nil && goEvent.Test != "" {
		status := archiveformat.TestStatus("")
		switch goEvent.Action {
		case "pass":
			status = archiveformat.TestStatusPassed
		case "fail":
			status = archiveformat.TestStatusFailed
		case "skip":
			status = archiveformat.TestStatusSkipped
		default:
			return archiveformat.TestResult{}, false
		}
		return archiveformat.TestResult{
			TestID: stableID("go/" + strings.TrimPrefix(goEvent.Package, "github.com/0xveya/tethux/") + "/" + goEvent.Test),
			Name:   goEvent.Test, Suite: "go", Status: status, Attempt: 1,
			Timing:     archiveformat.ResultTiming{DurationMS: int64(goEvent.Elapsed * 1000)},
			Parameters: map[string]any{"package": goEvent.Package},
			Artifacts:  []string{filepath.ToSlash(strings.TrimPrefix(sourcePath, stage+string(filepath.Separator)))},
		}, true
	}
	var generic map[string]any
	if json.Unmarshal(line, &generic) != nil {
		return archiveformat.TestResult{}, false
	}
	rawStatus, _ := generic["status"].(string)
	status := archiveformat.TestStatus(rawStatus)
	switch status {
	case archiveformat.TestStatusPassed, archiveformat.TestStatusFailed, archiveformat.TestStatusSkipped, archiveformat.TestStatusError, archiveformat.TestStatusCancelled:
	default:
		return archiveformat.TestResult{}, false
	}
	name := stringValue(generic, "name", "operation", "backend")
	if name == "" {
		name = "structured event"
	}
	id := stringValue(generic, "test_id")
	if id == "" {
		id = "event/" + name
	}
	return archiveformat.TestResult{
		TestID: stableID(id), Name: name, Suite: stringValue(generic, "suite", "schema"),
		Status: status, Attempt: 1,
		Timing:     archiveformat.ResultTiming{DurationMS: int64(numberValue(generic, "duration_ms"))},
		Parameters: generic,
		Artifacts:  []string{filepath.ToSlash(strings.TrimPrefix(sourcePath, stage+string(filepath.Separator)))},
	}, true
}

func (w *ArchiveWriter) buildManifest(results archiveformat.ResultsDocument) (archiveformat.Manifest, error) {
	files, err := collectArchiveFiles(w.Stage)
	if err != nil {
		return archiveformat.Manifest{}, err
	}
	var passed, failed, skipped, errored, cancelled int64
	for _, result := range results.Tests {
		switch result.Status {
		case archiveformat.TestStatusPassed:
			passed++
		case archiveformat.TestStatusFailed:
			failed++
		case archiveformat.TestStatusSkipped:
			skipped++
		case archiveformat.TestStatusCancelled:
			cancelled++
		default:
			errored++
		}
	}
	status := archiveformat.RunStatusPassed
	if w.Options.CommandErr != nil || errored > 0 {
		status = archiveformat.RunStatusError
	} else if failed > 0 {
		status = archiveformat.RunStatusFailed
	}
	hostname, _ := os.Hostname()
	branch, _ := gitOutput(w.Options.Repository, "branch", "--show-current")
	sourceType := archiveformat.SourceTypeLocal
	provider := ""
	if os.Getenv("CI") != "" {
		sourceType = archiveformat.SourceTypeCI
		provider = "woodpecker"
	}
	return archiveformat.Manifest{
		SchemaVersion: 1, RunID: w.Options.RunID,
		Project:     archiveformat.Project{ID: "tethux", Name: "tethux", Repository: "codeberg.org/tethux/tethux"},
		Source:      &archiveformat.Source{Type: sourceType, Provider: provider, Workflow: w.Options.Workflow, Job: w.Options.Workflow, Attempt: 1, Trigger: archiveformat.TriggerType(os.Getenv("CI_PIPELINE_EVENT"))},
		Git:         archiveformat.Git{CommitSHA: w.Options.Revision, Branch: branch},
		Timing:      archiveformat.Timing{StartedAt: w.Options.StartedAt, FinishedAt: w.Options.FinishedAt, DurationMS: w.Options.FinishedAt.Sub(w.Options.StartedAt).Milliseconds()},
		Runner:      archiveformat.Runner{DeviceID: firstNonEmpty(w.Options.DeviceID, hostname), DisplayName: firstNonEmpty(w.Options.DeviceID, hostname), Hostname: hostname, OS: runtime.GOOS, Architecture: runtime.GOARCH},
		Software:    archiveformat.Software{GoVersion: runtime.Version(), TestRunnerVersion: "2", ProjectBinaryVersion: "dev"},
		Environment: map[string]any{"privileged": os.Geteuid() == 0, "container_runtime": w.Options.Runtime},
		Summary:     archiveformat.Summary{Status: status, Total: int64(len(results.Tests)), Passed: passed, Failed: failed, Skipped: skipped, Errored: errored, Cancelled: cancelled},
		Files:       files, Labels: map[string]string{"runner_group": "declarative-go"},
	}, nil
}

func collectArchiveFiles(stage string) ([]archiveformat.ArchiveFile, error) {
	var files []archiveformat.ArchiveFile
	err := filepath.WalkDir(stage, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || entry.Name() == "manifest.json" {
			return err
		}
		relative, err := filepath.Rel(stage, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fileType := archiveformat.FileTypeArtifact
		relative = filepath.ToSlash(relative)
		switch {
		case relative == "results.json":
			fileType = archiveformat.FileTypeResults
		case strings.HasPrefix(relative, "logs/"):
			fileType = archiveformat.FileTypeLog
		case strings.HasPrefix(relative, "configs/"):
			fileType = archiveformat.FileTypeConfig
		case strings.HasSuffix(relative, ".pcap"), strings.HasSuffix(relative, ".pcapng"):
			fileType = archiveformat.FileTypePacketCapture
		}
		mediaType := mediaTypeForPath(relative)
		files = append(files, archiveformat.ArchiveFile{
			Path: relative, Type: fileType, MediaType: mediaType, SizeBytes: int64(len(content)),
			SHA256: fmt.Sprintf("%x", sha256.Sum256(content)), Public: fileType != archiveformat.FileTypePacketCapture,
		})
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, err
}

func writeJSONFile(path string, value any) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encodeErr := encoder.Encode(value)
	closeErr := file.Close()
	return errors.Join(encodeErr, closeErr)
}

func writeTarZstd(destination, stage string) error {
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	encoder, err := zstd.NewWriter(file)
	if err != nil {
		file.Close()
		return err
	}
	tw := tar.NewWriter(encoder)
	walkErr := filepath.WalkDir(stage, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		relative, err := filepath.Rel(stage, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relative)
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		source, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tw, source)
		closeErr := source.Close()
		return errors.Join(copyErr, closeErr)
	})
	return errors.Join(walkErr, tw.Close(), encoder.Close(), file.Close())
}

func gitOutput(root string, args ...string) (string, error) {
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(output)), nil
}

func stableID(value string) string {
	value = strings.ToLower(value)
	var result strings.Builder
	lastSeparator := false
	for _, char := range value {
		valid := char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '.' || char == '/'
		if valid {
			result.WriteRune(char)
			lastSeparator = false
		} else if !lastSeparator {
			result.WriteByte('-')
			lastSeparator = true
		}
	}
	return strings.Trim(result.String(), "-/")
}

func stringValue(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}

func numberValue(values map[string]any, key string) float64 {
	value, _ := values[key].(float64)
	return value
}

func mediaTypeForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return "application/json"
	case ".jsonl":
		return "application/x-ndjson"
	case ".yaml", ".yml":
		return "application/yaml"
	case ".pcap", ".pcapng":
		return "application/vnd.tcpdump.pcap"
	default:
		return "text/plain"
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
