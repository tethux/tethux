package ingest

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/0xveya/tethux/internal/ciresults/db"
	dbgen "github.com/0xveya/tethux/internal/ciresults/db/sqlc"
	"github.com/0xveya/tethux/internal/ciresults/ingest/archiveformat"
)

func persistRun(ctx context.Context, store *db.Store, record IngestionRecord, manifest archiveformat.Manifest) (returnErr error) {
	results, err := DecodeResults(bytes.NewReader(record.ResultsJSON))
	if err != nil {
		return err
	}
	if results.RunID != manifest.RunID || manifest.RunID != record.RunID {
		return fmt.Errorf("run IDs do not match: archive=%q manifest=%q results=%q", record.RunID, manifest.RunID, results.RunID)
	}

	tx, err := store.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin ingestion transaction: %w", err)
	}
	defer func() {
		if returnErr != nil {
			_ = tx.Rollback()
		}
	}()
	q := dbgen.New(tx)

	exists, err := q.RunUIDExists(ctx, record.RunID)
	if err != nil {
		return fmt.Errorf("check existing run: %w", err)
	}
	if exists {
		return tx.Rollback()
	}

	relativePath := filepath.ToSlash(filepath.Join(record.Hash, record.Variant.String(), filepath.Base(record.ArchivePath)))
	var size int64
	var mtime sql.NullInt64
	if info, statErr := os.Stat(record.ArchivePath); statErr == nil {
		size = info.Size()
		mtime = sql.NullInt64{Int64: info.ModTime().UnixNano(), Valid: true}
	}
	archive, err := q.CreateImportingArchive(ctx, dbgen.CreateImportingArchiveParams{
		RelativePath: relativePath, FileSizeBytes: size, FileMtimeNs: mtime,
	})
	if err != nil {
		return fmt.Errorf("create importing archive: %w", err)
	}

	project, err := q.UpsertProject(ctx, dbgen.UpsertProjectParams{
		ProjectKey: manifest.Project.ID, Name: nullString(manifest.Project.Name), Repository: nullString(manifest.Project.Repository),
	})
	if err != nil {
		return fmt.Errorf("upsert project: %w", err)
	}
	device, err := q.UpsertDevice(ctx, dbgen.UpsertDeviceParams{
		DeviceKey: manifest.Runner.DeviceID, DisplayName: nullString(manifest.Runner.DisplayName),
		LastOs: nullString(manifest.Runner.OS), LastOsVersion: nullString(manifest.Runner.OSVersion),
		LastKernel: nullString(manifest.Runner.Kernel), LastArch: nullString(manifest.Runner.Architecture),
		LastCpu: nullString(manifest.Runner.CPU), LastMemoryBytes: nullInt64(manifest.Runner.MemoryBytes),
		MetadataJson: "{}", SeenAt: nullString(manifest.Timing.FinishedAt.Format(timeFormat)),
	})
	if err != nil {
		return fmt.Errorf("upsert device: %w", err)
	}

	software, err := marshalJSON(manifest.Software)
	if err != nil {
		return err
	}
	environment, err := marshalJSON(manifest.Environment)
	if err != nil {
		return err
	}
	labels, err := marshalJSON(manifest.Labels)
	if err != nil {
		return err
	}
	var source archiveformat.Source
	if manifest.Source != nil {
		source = *manifest.Source
	}
	attempt := int64(source.Attempt)
	if attempt < 1 {
		attempt = 1
	}
	run, err := q.CreateRun(ctx, dbgen.CreateRunParams{
		RunUid: manifest.RunID, SchemaVersion: int64(manifest.SchemaVersion), ArchiveID: archive.ID,
		ProjectID: project.ID, DeviceID: device.ID, SourceType: nullString(string(source.Type)),
		SourceProvider: nullString(source.Provider), Workflow: nullString(source.Workflow), Job: nullString(source.Job),
		TriggerName: nullString(string(source.Trigger)), SourceAttempt: attempt, CommitSha: manifest.Git.CommitSHA,
		Branch: nullString(manifest.Git.Branch), Tag: nullStringPointer(manifest.Git.Tag), GitDirty: nullBool(manifest.Git.Dirty),
		CommitTimestamp: nullTime(manifest.Git.CommitTimestamp), StartedAt: manifest.Timing.StartedAt.Format(timeFormat),
		FinishedAt: manifest.Timing.FinishedAt.Format(timeFormat), DurationMs: manifest.Timing.DurationMS,
		Status: string(manifest.Summary.Status), TotalCount: manifest.Summary.Total, PassedCount: manifest.Summary.Passed,
		FailedCount: manifest.Summary.Failed, SkippedCount: manifest.Summary.Skipped, ErroredCount: manifest.Summary.Errored,
		CancelledCount: manifest.Summary.Cancelled, SoftwareJson: software, EnvironmentJson: environment,
		LabelsJson: labels, ManifestJson: string(record.ManifestJSON),
	})
	if err != nil {
		return fmt.Errorf("create run: %w", err)
	}

	fileIDs := make(map[string]int64, len(manifest.Files))
	for _, file := range manifest.Files {
		stored, err := q.CreateArchiveFile(ctx, dbgen.CreateArchiveFileParams{
			RunID: run.ID, ArchivePath: file.Path, FileType: string(file.Type), MediaType: file.MediaType,
			SizeBytes: file.SizeBytes, Sha256: file.SHA256, IsPublic: boolInt(file.Public),
		})
		if err != nil {
			return fmt.Errorf("create archive file %q: %w", file.Path, err)
		}
		fileIDs[file.Path] = stored.ID
	}

	for _, test := range results.Tests {
		testCase, err := q.UpsertTestCase(ctx, dbgen.UpsertTestCaseParams{
			ProjectID: project.ID, TestKey: test.TestID, Name: test.Name, Suite: nullString(test.Suite), ResultKind: "go_test",
			SourceFile: resultSourceFile(test.Source), SourceSymbol: resultSourceSymbol(test.Source),
			FirstSeenAt: nullTime(test.Timing.StartedAt), LastSeenAt: nullTime(test.Timing.FinishedAt),
		})
		if err != nil {
			return fmt.Errorf("upsert test case %q: %w", test.TestID, err)
		}
		parameters, err := marshalJSON(test.Parameters)
		if err != nil {
			return err
		}
		metrics, err := marshalJSON(test.Metrics)
		if err != nil {
			return err
		}
		testLabels, err := marshalJSON(test.Labels)
		if err != nil {
			return err
		}
		var failure archiveformat.Failure
		if test.Failure != nil {
			failure = *test.Failure
		}
		result, err := q.CreateTestResult(ctx, dbgen.CreateTestResultParams{
			RunID: run.ID, TestCaseID: testCase.ID, Attempt: test.Attempt, Status: string(test.Status),
			StartedAt: nullTime(test.Timing.StartedAt), FinishedAt: nullTime(test.Timing.FinishedAt),
			DurationMs: sql.NullInt64{Int64: test.Timing.DurationMS, Valid: true}, Message: nullStringPointer(test.Message),
			FailureKind: nullString(string(failure.Kind)), FailurePhase: nullString(failure.Phase), FailureCode: nullString(failure.ErrorCode),
			ExpectedValue: nullStringPointer(failure.Expected), ActualValue: nullStringPointer(failure.Actual),
			StackTrace: nullStringPointer(failure.StackTrace), ParametersJson: parameters, MetricsJson: metrics,
			LabelsJson: testLabels, DetailsJson: "{}",
		})
		if err != nil {
			return fmt.Errorf("create test result %q: %w", test.TestID, err)
		}
		for _, featureKey := range test.Features {
			feature, err := q.UpsertFeature(ctx, dbgen.UpsertFeatureParams{ProjectID: project.ID, FeatureKey: featureKey})
			if err != nil {
				return fmt.Errorf("upsert feature %q: %w", featureKey, err)
			}
			if err := q.LinkTestFeature(ctx, dbgen.LinkTestFeatureParams{TestCaseID: testCase.ID, FeatureID: feature.ID}); err != nil {
				return err
			}
		}
		for _, path := range test.Artifacts {
			if fileID, ok := fileIDs[path]; ok {
				if err := q.LinkResultFile(ctx, dbgen.LinkResultFileParams{ResultID: result.ID, ArchiveFileID: fileID, Relationship: "artifact"}); err != nil {
					return err
				}
			}
		}
	}
	if err := q.MarkArchiveImported(ctx, archive.ID); err != nil {
		return fmt.Errorf("mark archive imported: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit ingestion transaction: %w", err)
	}
	return nil
}

const timeFormat = "2006-01-02T15:04:05.999999999Z07:00"

func marshalJSON(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode JSON: %w", err)
	}
	return string(data), nil
}

func nullString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

func nullStringPointer(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *value, Valid: true}
}

func nullTime(value *time.Time) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: value.Format(timeFormat), Valid: true}
}

func nullInt64(value *int64) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *value, Valid: true}
}

func nullBool(value *bool) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	if *value {
		return sql.NullInt64{Int64: 1, Valid: true}
	}
	return sql.NullInt64{Valid: true}
}

func boolInt(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

func resultSourceFile(value *archiveformat.ResultSource) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return nullString(value.File)
}

func resultSourceSymbol(value *archiveformat.ResultSource) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return nullStringPointer(value.Symbol)
}
