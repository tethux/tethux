// Package storage defines the provider-independent storage model used by
// Tethux workloads.
//
// The model has two deliberately separate layers:
//
//   - Provider is durable object storage. It addresses objects with Ref values
//     and provides operations such as Stat, Open, Put, List, Copy, Move, and
//     Delete.
//   - Manager prepares storage for a workload. It turns a durable Ref into a
//     Prepared runtime resource, tracks writes to that resource, commits dirty
//     changes, and releases the preparation.
//
// A provider owns durable objects. A manager owns the lifecycle of runtime
// material used by a workload. A Provider must not choose placement, staging,
// writeback timing, or workload cleanup merely because it also implements
// Manager.
//
// The usual lifecycle is:
//
//	Ref -> Manager.Prepare -> Prepared.Location -> workload
//	                                   |
//	                                   v
//	                           Manager.MarkDirty
//	                                   |
//	                                   v
//	                           Manager.Commit
//	                                   |
//	                                   v
//	                          durable Ref generation
//
// A Ref identifies one object as ProviderName:Key. The provider name selects
// the backend; the key is provider-relative and is not a host filesystem path.
// Ref values are the stable identity used in requests, events, and prepared
// records. ObjectInfo describes the current durable object, including its type,
// size, modification time, generation, content metadata, and artifact kind.
//
// A Generation is an opaque provider value. Callers must compare generations
// for equality and must not parse or manufacture them. Prepare records the
// generation from which runtime material was created. Commit may use
// CommitOptions.ExpectedGeneration to reject a stale write with a conflict.
// A successful commit returns the resulting ObjectInfo and emits a committed
// event with its new generation.
//
// PrepareRequest.Mode selects how runtime storage is materialized:
//
//   - PrepareDirect exposes the durable object directly.
//   - PrepareCopy creates independent runtime material.
//   - PrepareOverlay creates writable material backed by another object.
//
// The provider may reject modes it does not support. Prepared.ResourceType
// describes the runtime resource, Prepared.Location describes how the workload
// reaches it, and Prepared.Ownership describes who owns that runtime material.
// Ownership of runtime material does not transfer ownership of the durable Ref.
// In particular, Release must not delete a durable object solely because a
// prepared runtime resource is node-owned.
//
// Writeback is explicit. A workload that changes writable prepared storage calls
// MarkDirty. MarkDirty is idempotent. Commit writes the prepared resource back
// to the durable object, optionally checks ExpectedGeneration, applies the
// requested durability barrier, clears dirty state, and returns the resulting
// object. Read-only preparations cannot be marked dirty.
//
// Asynchronous methods return operation handles. An operation has a status,
// cancellation, wait, and event stream. PrepareOperation additionally exposes
// Prepared, while CommitOperation additionally exposes Result. Cancellation is
// cooperative and is reported through the operation state and its events.
//
// EventSource provides lifecycle notifications for prepared, dirty, committed,
// and released resources. Event streams are bounded and may drop notifications
// when a subscriber does not keep up. Operation events describe operation
// lifecycle; manager events describe storage lifecycle. API errors are returned
// directly and retain provider-specific structured categories where available.
package storage
