package ingest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var gitSHA1 = regexp.MustCompile(`^[0-9a-f]{40}$`)

const archiveSuffix = ".tar.zst"

func DiscoverCandidates(ctx context.Context, root string) ([]ArchiveRef, error) {
	hashEntries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read ingestion root %q: %w", root, err)
	}

	var runs []ArchiveRef

	for _, hashEntry := range hashEntries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if !hashEntry.IsDir() || !gitSHA1.MatchString(hashEntry.Name()) {
			continue
		}

		hash := hashEntry.Name()
		hashPath := filepath.Join(root, hash)

		variantEntries, err := os.ReadDir(hashPath)
		if err != nil {
			return nil, fmt.Errorf("read hash directory %q: %w", hashPath, err)
		}

		for _, variantEntry := range variantEntries {
			if !variantEntry.IsDir() {
				continue
			}

			variant, err := ParseVariant(variantEntry.Name())
			if err != nil {
				continue
			}

			variantPath := filepath.Join(hashPath, variantEntry.Name())

			archiveEntries, err := os.ReadDir(variantPath)
			if err != nil {
				return nil, fmt.Errorf(
					"read variant directory %q: %w",
					variantPath,
					err,
				)
			}

			for _, archiveEntry := range archiveEntries {
				if err := ctx.Err(); err != nil {
					return nil, err
				}

				if archiveEntry.IsDir() {
					continue
				}

				filename := archiveEntry.Name()
				if !strings.HasSuffix(filename, archiveSuffix) {
					continue
				}

				runID := strings.TrimSuffix(filename, archiveSuffix)
				if !IsUUIDv7(runID) {
					continue
				}

				archivePath := filepath.Join(variantPath, filename)

				runs = append(runs, ArchiveRef{
					Hash:  hash,
					RunID: runID,
					Variants: []VariantDetails{
						{
							Variant:     variant,
							ArchivePath: archivePath,
						},
					},
				},
				)
			}
		}
	}

	return runs, nil
}
