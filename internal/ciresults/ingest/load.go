package ingest

import (
	"context"
	"fmt"
	"os"
)

func GetData(ctx context.Context, run ExtractedRun) (*IngestionRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	manifestJSON, err := os.ReadFile(run.ManifestPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest %q: %w", run.ManifestPath, err)
	}

	resultsJSON, err := os.ReadFile(run.ResultsPath)
	if err != nil {
		return nil, fmt.Errorf("read results %q: %w", run.ResultsPath, err)
	}

	archivePath := ""
	for _, details := range run.Archive.Variants {
		if details.Variant == run.Variant {
			archivePath = details.ArchivePath
			break
		}
	}

	return &IngestionRecord{
		Hash:        run.Archive.Hash,
		RunID:       run.Archive.RunID,
		Variant:     run.Variant,
		ArchivePath: archivePath,

		ManifestJSON: manifestJSON,
		ResultsJSON:  resultsJSON,

		ConfigsDir:   run.ConfigsDir,
		LogsDir:      run.LogsDir,
		ArtifactsDir: run.ArtifactsDir,
	}, nil
}
