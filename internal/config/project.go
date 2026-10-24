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
	// KeyProjects holds the named projects.
	KeyProjects = "projects"
	// KeyCurrentProject is the active project name.
	KeyCurrentProject = "current_project"
	// KeyPreviousProject is the project active before the last switch, used by `project -`.
	KeyPreviousProject = "previous_project"
	// KeyAliases holds user-defined command aliases.
	KeyAliases = "aliases"
	// KeySavedQueries holds named JQL queries.
	KeySavedQueries = "saved_queries"
)

// projectKeys are the per-project settings a project may override. Everything
// else (server, login, auth, epic and custom field ids) is server-wide.
var projectKeys = []string{"project", "board", "issue", "sprint", "jql"}

var validName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

// ValidateProjectName rejects names that would not round-trip as YAML keys
// or that collide with the `project -` shorthand.
func ValidateProjectName(name string) error {
	if !validName.MatchString(name) {
		return fmt.Errorf("invalid project name %q: use letters, digits, '.', '_' or '-'", name)
	}
	return nil
}

// ActiveProject returns the project to use: override (flag/env) if set,
// otherwise current_project from the config.
func ActiveProject(v *viper.Viper, override string) string {
	if override != "" {
		return override
	}
	return v.GetString(KeyCurrentProject)
}

// ApplyProject overlays projects.<name> onto the top-level config so every
// existing command (which reads project.key, board.id, issue.types, ...)
// transparently uses the project. Project keys replace the top-level keys
// wholesale rather than deep-merging, so a project without e.g. board.type
// does not inherit the default board's type. The overlay is loaded into
// viper's config layer, so command-line flags such as --project still win.
func ApplyProject(v *viper.Viper, name string) error {
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

	projects, _ := cfg[KeyProjects].(map[string]any)
	entry, ok := projects[name]
	if !ok {
		return fmt.Errorf("project %q not found (see 'jt project ls')", name)
	}
	values, _ := entry.(map[string]any)
	for _, k := range projectKeys {
		if val, ok := values[k]; ok {
			cfg[k] = val
		}
	}
	cfg[KeyCurrentProject] = name

	merged, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	v.SetConfigType(FileType)
	return v.ReadConfig(bytes.NewReader(merged))
}

// SavedProject is a summary of a configured project.
type SavedProject struct {
	Name      string
	Key       string // Jira project key
	Board     string
	BoardID   string
	BoardType string
	Current   bool
}

// Projects lists the saved projects defined in the file.
func (f *File) Projects() []SavedProject {
	current := f.GetString(KeyCurrentProject)
	names := f.Keys(KeyProjects)
	out := make([]SavedProject, 0, len(names))
	for _, n := range names {
		out = append(out, SavedProject{
			Name:      n,
			Key:       f.GetString(KeyProjects, n, "project", "key"),
			Board:     f.GetString(KeyProjects, n, "board", "name"),
			BoardID:   f.GetString(KeyProjects, n, "board", "id"),
			BoardType: f.GetString(KeyProjects, n, "board", "type"),
			Current:   n == current,
		})
	}
	return out
}

// Resolve maps a selector to a saved project name: an exact name wins,
// then a case-insensitive Jira key match. It returns "" if nothing matches.
func (f *File) Resolve(selector string) string {
	if selector == "" {
		return ""
	}
	if f.HasProject(selector) {
		return selector
	}
	for _, p := range f.Projects() {
		if strings.EqualFold(p.Key, selector) {
			return p.Name
		}
	}
	return ""
}

// HasProject reports whether name is defined.
func (f *File) HasProject(name string) bool {
	return f.Get(KeyProjects, name) != nil
}

// UseProject makes name the current project and remembers the previous one.
// The special name "-" swaps back to the previous project.
func (f *File) UseProject(name string) (string, error) {
	current := f.GetString(KeyCurrentProject)
	if name == "-" {
		name = f.GetString(KeyPreviousProject)
		if name == "" {
			return "", fmt.Errorf("no previous project")
		}
	}
	if !f.HasProject(name) {
		return "", fmt.Errorf("project %q not found (see 'jt project ls')", name)
	}
	if name == current {
		return name, nil
	}
	if err := f.Set(name, KeyCurrentProject); err != nil {
		return "", err
	}
	if current != "" {
		if err := f.Set(current, KeyPreviousProject); err != nil {
			return "", err
		}
	}
	return name, nil
}

// DeleteProject removes a project. Deleting the current project clears
// current_project so commands fall back to the top-level project.
func (f *File) DeleteProject(name string) error {
	if !f.Delete(KeyProjects, name) {
		return fmt.Errorf("project %q not found", name)
	}
	if f.GetString(KeyCurrentProject) == name {
		f.Delete(KeyCurrentProject)
	}
	if f.GetString(KeyPreviousProject) == name {
		f.Delete(KeyPreviousProject)
	}
	return nil
}

// RenameProject renames a project, keeping current/previous pointers valid.
func (f *File) RenameProject(from, to string) error {
	if err := ValidateProjectName(to); err != nil {
		return err
	}
	if f.HasProject(to) {
		return fmt.Errorf("project %q already exists", to)
	}
	n := f.Get(KeyProjects)
	if n == nil {
		return fmt.Errorf("project %q not found", from)
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == from {
			n.Content[i].Value = to
			for _, k := range []string{KeyCurrentProject, KeyPreviousProject} {
				if f.GetString(k) == from {
					if err := f.Set(to, k); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}
	return fmt.Errorf("project %q not found", from)
}

// DefaultProjectName derives a project name from a project key.
func DefaultProjectName(projectKey string) string {
	return strings.ToLower(projectKey)
}
