package storage

// ChecksumAlgorithm identifies how a checksum was calculated.
type ChecksumAlgorithm string

const (
	// ChecksumSHA256 identifies a SHA-256 checksum.
	ChecksumSHA256 ChecksumAlgorithm = "sha256"
)

// Checksum identifies an object checksum and its algorithm.
type Checksum struct {
	Algorithm ChecksumAlgorithm
	Value     string
}

// Metadata contains provider-independent object metadata.
type Metadata map[string]string

// ArtifactKind describes the semantic purpose of an object.
type ArtifactKind string

const (
	// ArtifactGeneric identifies an object without a more specific purpose.
	ArtifactGeneric ArtifactKind = "generic"

	// ArtifactDisk identifies a VM or workload disk.
	ArtifactDisk ArtifactKind = "disk"

	// ArtifactISO identifies an ISO image.
	ArtifactISO ArtifactKind = "iso"

	// ArtifactImage identifies a generic boot or runtime image.
	ArtifactImage ArtifactKind = "image"

	// ArtifactConfig identifies a configuration artifact.
	ArtifactConfig ArtifactKind = "config"

	// ArtifactSnapshot identifies a storage snapshot.
	ArtifactSnapshot ArtifactKind = "snapshot"

	// ArtifactExport identifies an exported archive or artifact.
	ArtifactExport ArtifactKind = "export"
)
