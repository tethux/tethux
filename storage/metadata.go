package storage

// ChecksumAlgorithm identifies how a checksum was calculated.
type ChecksumAlgorithm string

const (
	ChecksumSHA256 ChecksumAlgorithm = "sha256"
)

// Checksum identifies object content by checksum.
type Checksum struct {
	Algorithm ChecksumAlgorithm
	Value     string
}

// Metadata contains provider-independent artifact metadata.
type Metadata map[string]string

// ArtifactKind describes the semantic purpose of a durable object.
type ArtifactKind string

const (
	ArtifactGeneric  ArtifactKind = "generic"
	ArtifactDisk     ArtifactKind = "disk"
	ArtifactISO      ArtifactKind = "iso"
	ArtifactImage    ArtifactKind = "image"
	ArtifactConfig   ArtifactKind = "config"
	ArtifactSnapshot ArtifactKind = "snapshot"
	ArtifactExport   ArtifactKind = "export"
)
