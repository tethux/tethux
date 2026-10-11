package dynamips

// RuntimeConfig contains router settings with a prepared native state directory.
type RuntimeConfig struct {
	Config
	// StateDirectory is an existing absolute directory exclusively leased to this router.
	// Empty preserves legacy working-directory filenames for direct provider callers.
	StateDirectory string
}
