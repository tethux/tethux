package ingest

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
)

func ExtractCandidate(
	ctx context.Context,
	candidate ArchiveRef,
) (*ExtractedCandidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	tmpDir, err := os.MkdirTemp("", "tethux-ingest-*")
	if err != nil {
		return nil, fmt.Errorf("create temp directory: %w", err)
	}

	result := &ExtractedCandidate{
		TempDir: tmpDir,
		Runs:    make([]ExtractedRun, 0, len(candidate.Variants)),
	}

	success := false
	defer func() {
		if !success {
			_ = result.Close()
		}
	}()

	for _, details := range candidate.Variants {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		variantDir := filepath.Join(tmpDir, details.Variant.String())

		if err := os.MkdirAll(variantDir, 0o755); err != nil {
			return nil, fmt.Errorf(
				"create variant directory %q: %w",
				details.Variant,
				err,
			)
		}

		if err := extractTarZst(details.ArchivePath, variantDir); err != nil {
			return nil, fmt.Errorf(
				"extract %s: %w",
				details.Variant,
				err,
			)
		}

		runDir, err := findRunDir(variantDir, candidate.RunID)
		if err != nil {
			return nil, fmt.Errorf(
				"find run directory for %s: %w",
				details.Variant,
				err,
			)
		}

		result.Runs = append(result.Runs, ExtractedRun{
			Archive: candidate,
			TempDir: tmpDir,
			Variant: details.Variant,

			ManifestPath: filepath.Join(runDir, "manifest.json"),
			ResultsPath:  filepath.Join(runDir, "results.json"),
			ConfigsDir:   filepath.Join(runDir, "configs"),
			LogsDir:      filepath.Join(runDir, "logs"),
			ArtifactsDir: filepath.Join(runDir, "artifacts"),
		})
	}

	success = true
	return result, nil
}

func findRunDir(variantDir, expectedRunID string) (string, error) {
	if fileExists(filepath.Join(variantDir, "manifest.json")) {
		return variantDir, nil
	}

	if expectedRunID != "" {
		expected := filepath.Join(variantDir, expectedRunID)

		info, err := os.Stat(expected)
		if err == nil && info.IsDir() {
			return expected, nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}

	entries, err := os.ReadDir(variantDir)
	if err != nil {
		return "", err
	}

	var runDir string

	for _, entry := range entries {
		if !entry.IsDir() || !IsUUIDv7(entry.Name()) {
			continue
		}

		if runDir != "" {
			return "", fmt.Errorf(
				"multiple UUIDv7 run directories found in %q",
				variantDir,
			)
		}

		runDir = filepath.Join(variantDir, entry.Name())
	}

	if runDir == "" {
		return "", fmt.Errorf(
			"no extracted UUIDv7 run directory found in %q",
			variantDir,
		)
	}

	return runDir, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func extractTarZst(archivePath, destDir string) error {
	archive, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer archive.Close()

	zr, err := zstd.NewReader(archive)
	if err != nil {
		return fmt.Errorf("create zstd reader: %w", err)
	}
	defer zr.Close()

	return extractTar(zr, destDir)
}

func extractTar(r io.Reader, destDir string) error {
	root, err := filepath.Abs(destDir)
	if err != nil {
		return fmt.Errorf("resolve destination: %w", err)
	}

	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("create destination: %w", err)
	}

	tr := tar.NewReader(r)

	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read tar entry: %w", err)
		}

		target, err := safeArchivePath(root, header.Name)
		if err != nil {
			return err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("create directory %q: %w", target, err)
			}

		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("create parent directory: %w", err)
			}

			file, err := os.OpenFile(
				target,
				os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
				os.FileMode(header.Mode),
			)
			if err != nil {
				return fmt.Errorf("create file %q: %w", target, err)
			}

			_, copyErr := io.Copy(file, tr)
			closeErr := file.Close()

			if copyErr != nil {
				return fmt.Errorf("extract %q: %w", target, copyErr)
			}
			if closeErr != nil {
				return fmt.Errorf("close %q: %w", target, closeErr)
			}

		case tar.TypeSymlink, tar.TypeLink:
			return fmt.Errorf("archive links are not supported: %q", header.Name)
		}
	}
}

func safeArchivePath(root, name string) (string, error) {
	cleanName := filepath.Clean(filepath.FromSlash(name))

	if filepath.IsAbs(cleanName) {
		return "", fmt.Errorf("absolute archive path: %q", name)
	}

	target := filepath.Join(root, cleanName)

	relative, err := filepath.Rel(root, target)
	if err != nil {
		return "", fmt.Errorf("validate path %q: %w", name, err)
	}

	if relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("archive path escapes destination: %q", name)
	}

	return target, nil
}
