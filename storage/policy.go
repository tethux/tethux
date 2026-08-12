package storage

// SyncPolicy controls how a commit makes prepared storage durable.
type SyncPolicy string

const (
	// SyncPolicyDefault lets the provider choose its normal durability policy.
	SyncPolicyDefault SyncPolicy = "default"

	// SyncPolicyNone requests no additional durability barrier.
	SyncPolicyNone SyncPolicy = "none"

	// SyncPolicyData requests that file contents are flushed to durable storage.
	SyncPolicyData SyncPolicy = "data"

	// SyncPolicyFull requests that file contents and relevant metadata are
	// flushed to durable storage.
	SyncPolicyFull SyncPolicy = "full"
)

// CommitOptions controls how a manager commits prepared storage.
//
// Providers may support only a subset of policies. Unsupported policies should
// be reported as errors rather than silently treated as another policy.
type CommitOptions struct {
	Sync SyncPolicy

	// ExpectedGeneration prevents committing over a newer object version.
	// An empty value means no generation check is requested.
	ExpectedGeneration Generation
}
