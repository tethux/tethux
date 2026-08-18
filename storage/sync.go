package storage

import "time"

// DurabilityPolicy controls durability guarantees requested for an individual
// write or commit.
type DurabilityPolicy string

const (
	// DurabilityDefault lets the implementation choose its normal behavior.
	DurabilityDefault DurabilityPolicy = "default"

	// DurabilityNone requests no additional durability barrier.
	DurabilityNone DurabilityPolicy = "none"

	// DurabilityData requests durable file contents.
	DurabilityData DurabilityPolicy = "data"

	// DurabilityFull requests durable file contents and relevant metadata.
	DurabilityFull DurabilityPolicy = "full"
)

// WritebackMode controls when dirty prepared storage should be committed.
//
// This is orchestration policy. It is intentionally separate from
// DurabilityPolicy.
type WritebackMode string

const (
	// WritebackManual commits only when explicitly requested.
	WritebackManual WritebackMode = "manual"

	// WritebackOnStop commits when the owning workload reaches the relevant
	// stopped lifecycle boundary.
	WritebackOnStop WritebackMode = "on-stop"

	// WritebackPeriodic periodically commits dirty runtime storage.
	WritebackPeriodic WritebackMode = "periodic"
)

// WritebackPolicy describes automatic commit behavior.
//
// Interval is required for WritebackPeriodic and ignored for other modes.
type WritebackPolicy struct {
	Mode WritebackMode

	Interval time.Duration

	Durability DurabilityPolicy
}

// CommitOptions controls one commit from prepared runtime storage to its
// durable object.
type CommitOptions struct {
	Durability DurabilityPolicy

	// ExpectedGeneration prevents overwriting a durable object that has changed
	// since it was prepared.
	//
	// An empty generation disables the check.
	ExpectedGeneration Generation
}

// CommitResult describes the durable object after a successful commit.
type CommitResult struct {
	Object ObjectInfo
}
