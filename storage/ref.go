package storage

import (
	"fmt"

	storageerrs "github.com/tethux/tethux/storage/errs"
)

// ProviderName identifies a registered storage provider.
type ProviderName string

// Key identifies an object within a storage provider.
type Key string

// Ref identifies an object by provider and provider-relative key.
type Ref struct {
	Provider ProviderName
	Key      Key
}

// Validate reports whether the reference has a provider and key.
func (r Ref) Validate() error {
	if r.Provider == "" {
		return storageerrs.New("storage", storageerrs.ErrInvalidRef, "provider is empty")
	}
	if r.Key == "" {
		return storageerrs.New("storage", storageerrs.ErrInvalidRef, "key is empty")
	}
	return nil
}

// String returns the reference in provider:key form.
func (r Ref) String() string {
	return fmt.Sprintf("%s:%s", r.Provider, r.Key)
}
