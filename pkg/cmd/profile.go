package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"sigs.k8s.io/yaml"
)

// contextsKey is the profile document key holding the contexts; a
// consumer's ProfileField may not use it.
const contextsKey = "contexts"

// ProfileFile is the profiles configuration document (the AWS profile /
// gcloud configuration analog): named profiles plus the active selection.
type ProfileFile struct {
	Active   string              `json:"active,omitempty"`
	Profiles map[string]*Profile `json:"profiles,omitempty"`
}

// Profile is one named profile. It answers two questions: where and who —
// the consumer's settings (endpoints, the auth issuer), declared by its
// ProfileFields — and which: the contexts, one map of scope identifier
// defaults per namespace, keyed by identifier attribute ("tenant_id").
//
// Contexts live inside a profile, never beside it, so switching profile
// switches the scope defaults with it: a tenant id cannot be stored without
// saying which profile it belongs to.
//
// In the document the settings sit at the profile's top level beside
// "contexts":
//
//	endpoint: admin.example.com:443
//	contexts:
//	  identity:
//	    tenant_id: t-1
type Profile struct {
	Settings map[string]string
	Contexts map[string]map[string]string
}

// MarshalJSON flattens the settings beside the contexts.
func (p Profile) MarshalJSON() ([]byte, error) {

	doc := make(map[string]any, len(p.Settings)+1)
	for k, v := range p.Settings {
		doc[k] = v
	}
	if len(p.Contexts) > 0 {
		doc[contextsKey] = p.Contexts
	}
	return json.Marshal(doc)
}

// UnmarshalJSON reads the flattened form. A hand-edited scalar that is not a
// string (insecure: true) is kept in its text form.
func (p *Profile) UnmarshalJSON(data []byte) error {

	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		return err
	}

	*p = Profile{}
	for k, raw := range doc {
		if k == contextsKey {
			if err := json.Unmarshal(raw, &p.Contexts); err != nil {
				return fmt.Errorf("%s: %w", contextsKey, err)
			}
			continue
		}
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			var v any
			if err := json.Unmarshal(raw, &v); err != nil {
				return fmt.Errorf("%s: %w", k, err)
			}
			switch v.(type) {
			case map[string]any, []any:
				return fmt.Errorf("%s: a profile setting is a single value", k)
			}
			s = fmt.Sprint(v)
		}
		p.set(k, s)
	}
	return nil
}

// Setting returns one setting, "" when unset.
func (p *Profile) Setting(key string) string {
	if p == nil {
		return ""
	}
	return p.Settings[key]
}

func (p *Profile) set(key, value string) {
	if p.Settings == nil {
		p.Settings = map[string]string{}
	}
	p.Settings[key] = value
}

// Context returns a namespace's scope defaults, an empty map when none are
// stored.
func (p *Profile) Context(namespace string) map[string]string {
	if p == nil || p.Contexts[namespace] == nil {
		return map[string]string{}
	}
	return p.Contexts[namespace]
}

// SetContext stores one scope default in a namespace.
func (p *Profile) SetContext(namespace, attr, value string) {
	if p.Contexts == nil {
		p.Contexts = map[string]map[string]string{}
	}
	if p.Contexts[namespace] == nil {
		p.Contexts[namespace] = map[string]string{}
	}
	p.Contexts[namespace][attr] = value
}

// ClearContext removes the given attributes from a namespace, or the whole
// namespace when none are given.
func (p *Profile) ClearContext(namespace string, attrs ...string) {
	if len(attrs) == 0 {
		delete(p.Contexts, namespace)
		return
	}
	for _, attr := range attrs {
		delete(p.Contexts[namespace], attr)
	}
	if len(p.Contexts[namespace]) == 0 {
		delete(p.Contexts, namespace)
	}
}

// Names returns the defined profile names, sorted.
func (f *ProfileFile) Names() []string {
	names := make([]string, 0, len(f.Profiles))
	for name := range f.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Get returns the named profile; an unknown name is an error listing the
// known ones.
func (f *ProfileFile) Get(name string) (*Profile, error) {

	if p, ok := f.Profiles[name]; ok && p != nil {
		return p, nil
	}
	known := f.Names()
	if len(known) == 0 {
		return nil, fmt.Errorf("unknown profile %q: no profiles are defined", name)
	}
	return nil, fmt.Errorf("unknown profile %q: known profiles are %s", name, strings.Join(known, ", "))
}

// Add stores a new profile; a name already taken is an error.
func (f *ProfileFile) Add(name string, p *Profile) error {

	if name == "" {
		return fmt.Errorf("a profile needs a name")
	}
	if _, ok := f.Profiles[name]; ok {
		return fmt.Errorf("profile %q already exists: change it with update", name)
	}
	if f.Profiles == nil {
		f.Profiles = map[string]*Profile{}
	}
	f.Profiles[name] = p
	return nil
}

// Activate selects the named profile.
func (f *ProfileFile) Activate(name string) error {
	if _, err := f.Get(name); err != nil {
		return err
	}
	f.Active = name
	return nil
}

// Delete removes the named profile. The active profile is refused: delete
// would otherwise leave every later command pointing at nothing.
func (f *ProfileFile) Delete(name string) error {

	if _, err := f.Get(name); err != nil {
		return err
	}
	if name == f.Active {
		return fmt.Errorf("profile %q is active: activate another one first", name)
	}
	delete(f.Profiles, name)
	return nil
}

// Select returns the profile a command uses: the explicitly selected name
// (--profile, or its env variable) when given, the active one otherwise.
// With neither, it returns "" and a nil profile, which reads as no settings
// and no context. An explicit name that does not exist is an error.
func (f *ProfileFile) Select(explicit string) (string, *Profile, error) {

	name := explicit
	if name == "" {
		name = f.Active
	}
	if name == "" {
		return "", nil, nil
	}
	p, err := f.Get(name)
	if err != nil {
		return "", nil, err
	}
	return name, p, nil
}

// ProfileStore loads and saves the profile file.
type ProfileStore struct {
	// Path is the file location, e.g. HomePath(".awctl.d", "config.yaml").
	Path string
}

// HomePath returns a path under the user's home directory.
func HomePath(elem ...string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(append([]string{home}, elem...)...), nil
}

// Load reads the profile file; a missing file is an empty document.
func (s ProfileStore) Load() (*ProfileFile, error) {

	data, err := os.ReadFile(s.Path)
	if os.IsNotExist(err) {
		return &ProfileFile{}, nil
	}
	if err != nil {
		return nil, err
	}

	f := &ProfileFile{}
	if err := yaml.Unmarshal(data, f); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", s.Path, err)
	}
	return f, nil
}

// Save writes the profile file, owner read/write only: it names installs,
// tenants and issuers, and sits beside the credential store.
func (s ProfileStore) Save(f *ProfileFile) error {

	data, err := yaml.Marshal(f)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(s.Path, data, 0o600)
}

// ProfileField declares one consumer setting a profile carries: cmdinfra
// knows no endpoints of its own, so the consumer lists them. The setting is
// stored under Key and set with --<Key in kebab case>.
type ProfileField struct {
	// Key is the document key and Profile.Settings key, snake_case
	// ("guard_endpoint").
	Key string
	// Usage is the flag's help text.
	Usage string
	// Bool makes the flag a boolean; the stored value is "true" or "false".
	Bool bool
}

// Flag is the field's flag name.
func (f ProfileField) Flag() string {
	return strings.ReplaceAll(f.Key, "_", "-")
}

// parseBool reads a stored boolean setting; anything but a true value is
// false.
func parseBool(s string) bool {
	b, _ := strconv.ParseBool(s)
	return b
}
