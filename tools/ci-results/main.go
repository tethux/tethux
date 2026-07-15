package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/0xveya/tethux/internal/ciresults/db"
	"github.com/0xveya/tethux/internal/ciresults/ingest"
	"github.com/0xveya/tethux/tools/ci-results/viewer"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "ingest":
		err = runIngestCommand(os.Args[2:])
	case "serve":
		err = runServeCommand(os.Args[2:])
	case "help", "-h", "--help":
		printUsage()
		return
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "ci-results: %v\n", err)
		os.Exit(1)
	}
}

func runIngestCommand(args []string) error {
	flags := flag.NewFlagSet("ingest", flag.ContinueOnError)
	root := flags.String("path", "./ingestion/archive", "path containing archived CI results")
	dbPath := flags.String("db", "data/ci/ci-res.db", "SQLite database path")
	verbose := flags.Bool("verbose", false, "print every decoded ingestion event")
	if err := flags.Parse(args); err != nil {
		return err
	}

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
	err := ingestPath(context.Background(), *root, *dbPath)
	if discard != nil {
		os.Stdout = output
		_ = discard.Close()
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(output, "ingestion complete path=%s db=%s elapsed=%s\n", *root, *dbPath, time.Since(started).Round(time.Millisecond))
	return nil
}

func runServeCommand(args []string) error {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	port := flags.Int("port", 8080, "HTTP port")
	dbPath := flags.String("db", "data/ci/ci-res.db", "SQLite database path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *port < 1 || *port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	return viewer.Serve(context.Background(), "127.0.0.1:"+strconv.Itoa(*port), *dbPath)
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "usage: ci-results <command> [options]")
	fmt.Fprintln(os.Stderr, "commands:")
	fmt.Fprintln(os.Stderr, "  ingest   load archived CI results into SQLite")
	fmt.Fprintln(os.Stderr, "  serve    serve the results API and web viewer")
}

func ingestPath(ctx context.Context, root, dbPath string) error {
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

	for runIndex := range extracted.Runs {
		extractedRun := &extracted.Runs[runIndex]
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

		record, err := ingest.GetData(ctx, *extractedRun)
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
