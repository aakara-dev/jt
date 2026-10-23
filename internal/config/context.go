package config

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

const (
	// KeyContexts holds the named contexts.
	KeyContexts = "contexts"
	// KeyCurrentContext is the active context name.
	KeyCurrentContext = "current_context"
	// KeyPreviousContext is the context active before the last switch, used by `ctx -`.
	KeyPreviousContext = "previous_context"
	// KeyAliases holds user-defined command aliases.
	KeyAliases = "aliases"
	// KeySavedQueries holds named JQL queries.
	KeySavedQueries = "saved_queries"
)

// contextKeys are the per-project settings a context may override. Everything
// else (server, login, auth, epic and custom field ids) is server-wide.
var contextKeys = []string{"project", "board", "issue", "sprint", "jql"}

var validName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

// ValidateContextName rejects names that would not round-trip as YAML keys
// or that collide with the `ctx -` shorthand.
func ValidateContextName(name string) error {
	if !validName.MatchString(name) {
		return fmt.Errorf("invalid context name %q: use letters, digits, '.', '_' or '-'", name)
	}
	return nil
}

// ActiveContext returns the context to use: override (flag/env) if set,
// otherwise current_context from the config.
func ActiveContext(v *viper.Viper, override string) string {
	if override != "" {
		return override
	}
	return v.GetString(KeyCurrentContext)
}

// ApplyContext overlays contexts.<name> onto the top-level config so every
// existing command (which reads project.key, board.id, issue.types, ...)
// transparently uses the context. Context keys replace the top-level keys
// wholesale rather than deep-merging, so a context without e.g. board.type
// does not inherit the default board's type. The overlay is loaded into
// viper's config layer, so command-line flags such as --project still win.
func ApplyContext(v *viper.Viper, name string) error {
	if name == "" {
		return nil
	}
	path := v.ConfigFileUsed()
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var cfg map[string]any
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	contexts, _ := cfg[KeyContexts].(map[string]any)
	ctx, ok := contexts[name]
	if !ok {
		return fmt.Errorf("context %q not found (see 'jt ctx ls')", name)
	}
	values, _ := ctx.(map[string]any)
	for _, k := range contextKeys {
		if val, ok := values[k]; ok {
			cfg[k] = val
		}
	}
	cfg[KeyCurrentContext] = name

	merged, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	v.SetConfigType(FileType)
	return v.ReadConfig(bytes.NewReader(merged))
}

// Context is a summary of a configured context.
type Context struct {
	Name      string
	Project   string
	Board     string
	BoardID   string
	BoardType string
	Current   bool
}

// Contexts lists the contexts defined in the file.
func (f *File) Contexts() []Context {
	current := f.GetString(KeyCurrentContext)
	names := f.Keys(KeyContexts)
	out := make([]Context, 0, len(names))
	for _, n := range names {
		out = append(out, Context{
			Name:      n,
			Project:   f.GetString(KeyContexts, n, "project", "key"),
			Board:     f.GetString(KeyContexts, n, "board", "name"),
			BoardID:   f.GetString(KeyContexts, n, "board", "id"),
			BoardType: f.GetString(KeyContexts, n, "board", "type"),
			Current:   n == current,
		})
	}
	return out
}

// HasContext reports whether name is defined.
func (f *File) HasContext(name string) bool {
	return f.Get(KeyContexts, name) != nil
}

// UseContext makes name the current context and remembers the previous one.
// The special name "-" swaps back to the previous context.
func (f *File) UseContext(name string) (string, error) {
	current := f.GetString(KeyCurrentContext)
	if name == "-" {
		name = f.GetString(KeyPreviousContext)
		if name == "" {
			return "", fmt.Errorf("no previous context")
		}
	}
	if !f.HasContext(name) {
		return "", fmt.Errorf("context %q not found (see 'jt ctx ls')", name)
	}
	if name == current {
		return name, nil
	}
	if err := f.Set(name, KeyCurrentContext); err != nil {
		return "", err
	}
	if current != "" {
		if err := f.Set(current, KeyPreviousContext); err != nil {
			return "", err
		}
	}
	return name, nil
}

// DeleteContext removes a context. Deleting the current context clears
// current_context so commands fall back to the top-level project.
func (f *File) DeleteContext(name string) error {
	if !f.Delete(KeyContexts, name) {
		return fmt.Errorf("context %q not found", name)
	}
	if f.GetString(KeyCurrentContext) == name {
		f.Delete(KeyCurrentContext)
	}
	if f.GetString(KeyPreviousContext) == name {
		f.Delete(KeyPreviousContext)
	}
	return nil
}

// RenameContext renames a context, keeping current/previous pointers valid.
func (f *File) RenameContext(from, to string) error {
	if err := ValidateContextName(to); err != nil {
		return err
	}
	if f.HasContext(to) {
		return fmt.Errorf("context %q already exists", to)
	}
	n := f.Get(KeyContexts)
	if n == nil {
		return fmt.Errorf("context %q not found", from)
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == from {
			n.Content[i].Value = to
			for _, k := range []string{KeyCurrentContext, KeyPreviousContext} {
				if f.GetString(k) == from {
					if err := f.Set(to, k); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}
	return fmt.Errorf("context %q not found", from)
}

// DefaultContextName derives a context name from a project key.
func DefaultContextName(projectKey string) string {
	return strings.ToLower(projectKey)
}
