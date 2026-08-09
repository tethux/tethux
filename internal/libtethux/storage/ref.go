package storage

import "fmt"

type ProviderName string

type Key string

type Ref struct {
	Provider ProviderName
	Key      Key
}

func (r Ref) Validate() error {
	if r.Provider == "" {
		return fmt.Errorf("storage provider is empty")
	}
	if r.Key == "" {
		return fmt.Errorf("storage key is empty")
	}
	return nil
}

func (r Ref) String() string {
	return fmt.Sprintf("%s:%s", r.Provider, r.Key)
}
