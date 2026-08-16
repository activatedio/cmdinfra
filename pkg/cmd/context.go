package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"sigs.k8s.io/yaml"
)

// ContextFile is the named-contexts configuration document (the gcloud
// configurations analog): named sets of scope identifier defaults plus the
// active selection.
type ContextFile struct {
	Active   string                       `json:"active,omitempty"`
	Contexts map[string]map[string]string `json:"contexts,omitempty"`
}

// ActiveValues returns the active context's values, or an empty map when
// nothing is active.
func (f *ContextFile) ActiveValues() map[string]string {
	if f == nil || f.Active == "" {
		return map[string]string{}
	}
	values, ok := f.Contexts[f.Active]
	if !ok {
		return map[string]string{}
	}
	return values
}

// Set stores a value in the named context, creating the context if needed.
func (f *ContextFile) Set(context, key, value string) {
	if f.Contexts == nil {
		f.Contexts = map[string]map[string]string{}
	}
	if f.Contexts[context] == nil {
		f.Contexts[context] = map[string]string{}
	}
	f.Contexts[context][key] = value
}

// Activate selects the named context; unknown names are errors listing the
// known ones.
func (f *ContextFile) Activate(name string) error {
	if _, ok := f.Contexts[name]; !ok {
		known := f.Names()
		if len(known) == 0 {
			return fmt.Errorf("unknown context %q: no contexts are defined", name)
		}
		return fmt.Errorf("unknown context %q: known contexts are %s", name, strings.Join(known, ", "))
	}
	f.Active = name
	return nil
}

// Names returns the defined context names, sorted.
func (f *ContextFile) Names() []string {
	names := make([]string, 0, len(f.Contexts))
	for name := range f.Contexts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ContextStore loads and saves the context file.
type ContextStore struct {
	// Path is the file location, e.g. HomeContextPath(".awctl.yaml").
	Path string
}

// HomeContextPath returns the given filename under the user's home
// directory.
func HomeContextPath(filename string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, filename), nil
}

// Load reads the context file; a missing file is an empty document.
func (s ContextStore) Load() (*ContextFile, error) {

	data, err := os.ReadFile(s.Path)
	if os.IsNotExist(err) {
		return &ContextFile{}, nil
	}
	if err != nil {
		return nil, err
	}

	f := &ContextFile{}
	if err := yaml.Unmarshal(data, f); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", s.Path, err)
	}
	return f, nil
}

// Save writes the context file (owner read/write only — it names tenants
// and issuers, and siblings may hold credentials).
func (s ContextStore) Save(f *ContextFile) error {

	data, err := yaml.Marshal(f)
	if err != nil {
		return err
	}
	return os.WriteFile(s.Path, data, 0o600)
}
