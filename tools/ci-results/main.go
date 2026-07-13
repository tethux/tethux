package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/0xveya/tethux/internal/ciresults/db"
	"github.com/0xveya/tethux/internal/ciresults/ingest"
)

func main() {
	root := flag.String(
		"path",
		"./ingestion/archive",
		"path containing the archived CI results",
	)
	dbPath := flag.String("db", "data/ci/ci-res.db", "SQLite database path")
	verbose := flag.Bool("verbose", false, "print every decoded ingestion event")

	flag.Parse()

	started := time.Now()
	output := os.Stdout
	var discard *os.File
	if !*verbose {
		var err error
		discard, err = os.OpenFile(os.DevNull, os.O_WRONLY, 0)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ci-results: open output sink: %v\n", err)
			os.Exit(1)
		}
		os.Stdout = discard
	}
	err := run(context.Background(), *root, *dbPath)
	if discard != nil {
		os.Stdout = output
		_ = discard.Close()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "ci-results: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(output, "ingestion complete path=%s db=%s elapsed=%s\n", *root, *dbPath, time.Since(started).Round(time.Millisecond))
}

func run(ctx context.Context, root, dbPath string) error {
	store, err := db.NewStore(dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	candidates, err := ingest.DiscoverCandidates(ctx, root)
	if err != nil {
		return fmt.Errorf("discover candidates: %w", err)
	}

	fmt.Printf("found %d candidate(s)\n", len(candidates))

	for candidateIndex, candidate := range candidates {
		fmt.Printf(
			"\n[%d/%d] archive %s / run %s / %d variant(s)\n",
			candidateIndex+1,
			len(candidates),
			candidate.Hash,
			candidate.RunID,
			len(candidate.Variants),
		)

		extracted, err := ingest.ExtractCandidate(ctx, candidate)
		if err != nil {
			return fmt.Errorf(
				"extract candidate %s/%s: %w",
				candidate.Hash,
				candidate.RunID,
				err,
			)
		}

		if err := processExtractedCandidate(ctx, store, extracted); err != nil {
			_ = extracted.Close()

			return fmt.Errorf(
				"process candidate %s/%s: %w",
				candidate.Hash,
				candidate.RunID,
				err,
			)
		}

		if err := extracted.Close(); err != nil {
			return fmt.Errorf(
				"remove extracted candidate %s/%s: %w",
				candidate.Hash,
				candidate.RunID,
				err,
			)
		}
	}

	return nil
}

func processExtractedCandidate(
	ctx context.Context,
	store *db.Store,
	extracted *ingest.ExtractedCandidate,
) error {
	fmt.Printf("extracted into %s\n", extracted.TempDir)

	for runIndex, extractedRun := range extracted.Runs {
		fmt.Printf(
			"  [%d/%d] variant=%s\n",
			runIndex+1,
			len(extracted.Runs),
			extractedRun.Variant,
		)

		fmt.Printf("    manifest:  %s\n", extractedRun.ManifestPath)
		fmt.Printf("    results:   %s\n", extractedRun.ResultsPath)
		fmt.Printf("    configs:   %s\n", extractedRun.ConfigsDir)
		fmt.Printf("    logs:      %s\n", extractedRun.LogsDir)
		fmt.Printf("    artifacts: %s\n", extractedRun.ArtifactsDir)

		record, err := ingest.GetData(ctx, extractedRun)
		if err != nil {
			return fmt.Errorf(
				"get data for variant %s: %w",
				extractedRun.Variant,
				err,
			)
		}

		fmt.Printf(
			"    loaded manifest=%d bytes results=%d bytes\n",
			len(record.ManifestJSON),
			len(record.ResultsJSON),
		)

		if err := ingest.IngestRun(
			ctx,
			store,
			*record,
		); err != nil {
			return fmt.Errorf(
				"run ingest variant %s: %w",
				extractedRun.Variant,
				err,
			)
		}
	}

	return nil
}
