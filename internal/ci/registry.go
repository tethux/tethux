package ci

import (
	"fmt"
	"sort"
	"sync"
)

type Provider struct {
	Name        string
	SourceType  string
	Environment map[string]string
}

type Registry struct {
	mu        sync.RWMutex
	workflows map[string]Workflow
	providers map[string]Provider
}

func NewRegistry() *Registry {
	return &Registry{workflows: map[string]Workflow{}, providers: map[string]Provider{}}
}

func (r *Registry) RegisterWorkflow(workflow Workflow) error {
	if err := ValidateWorkflow(workflow); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.workflows[workflow.Name]; exists {
		return fmt.Errorf("workflow %q is already registered", workflow.Name)
	}
	r.workflows[workflow.Name] = workflow
	return nil
}

func (r *Registry) Workflow(name string) (Workflow, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	workflow, ok := r.workflows[name]
	return workflow, ok
}

func (r *Registry) WorkflowNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.workflows))
	for name := range r.workflows {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (r *Registry) RegisterProvider(provider Provider) error {
	if provider.Name == "" {
		return fmt.Errorf("provider name is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.providers[provider.Name]; exists {
		return fmt.Errorf("provider %q is already registered", provider.Name)
	}
	r.providers[provider.Name] = provider
	return nil
}

func DefaultRegistry(root string) (*Registry, error) {
	registry := NewRegistry()
	if err := registry.RegisterProvider(Provider{
		Name: "woodpecker", SourceType: "ci",
		Environment: map[string]string{"CI_RESULTS_SOURCE_PROVIDER": "woodpecker"},
	}); err != nil {
		return nil, err
	}
	for _, workflow := range BuiltinWorkflows(root) {
		if err := registry.RegisterWorkflow(workflow); err != nil {
			return nil, err
		}
	}
	return registry, nil
}
