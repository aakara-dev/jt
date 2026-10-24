package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/spf13/viper"

	"github.com/aakara-dev/jt/internal/cmdutil"
	"github.com/aakara-dev/jt/pkg/env"
)

// The config sits next to credential settings (netrc/keyring references),
// so it is private to the user.
const (
	filePerm = 0o600
	dirPerm  = 0o700
)

// ErrNoConfig is returned when no config file exists at the resolved path.
var ErrNoConfig = errors.New("config file not found, run 'jt init' to create one")

// Path resolves the config file location: explicit flag value first, then
// JT_CONFIG_FILE (or JIRA_CONFIG_FILE), then ~/.config/jt/config.yml.
func Path(flagValue string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	if f := env.Get("CONFIG_FILE"); f != "" {
		return f, nil
	}
	home, err := cmdutil.GetConfigHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, Dir, FileName+"."+FileType), nil
}

// File is a YAML config file edited in place. It works on the yaml.Node tree
// so that comments and key order written by the user survive a Save.
type File struct {
	path string
	doc  *yaml.Node
}

// LoadFile reads the config file at path.
func LoadFile(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNoConfig
		}
		return nil, err
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if doc.Kind == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("parse %s: top level must be a mapping", path)
	}
	return &File{path: path, doc: &doc}, nil
}

// Path returns the file location.
func (f *File) Path() string { return f.path }

func (f *File) root() *yaml.Node { return f.doc.Content[0] }

// Get returns the node at the given key path, or nil.
func (f *File) Get(keys ...string) *yaml.Node {
	n := f.root()
	for _, k := range keys {
		if n == nil || n.Kind != yaml.MappingNode {
			return nil
		}
		n = mapValue(n, k)
	}
	return n
}

// GetString returns the scalar at the given key path, or "".
func (f *File) GetString(keys ...string) string {
	n := f.Get(keys...)
	if n == nil || n.Kind != yaml.ScalarNode {
		return ""
	}
	return n.Value
}

// Keys returns the sorted keys of the mapping at the given path.
func (f *File) Keys(keys ...string) []string {
	n := f.Get(keys...)
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	out := make([]string, 0, len(n.Content)/2)
	for i := 0; i < len(n.Content); i += 2 {
		out = append(out, n.Content[i].Value)
	}
	sort.Strings(out)
	return out
}

// StringMap returns the mapping at the given path as key/scalar pairs.
func (f *File) StringMap(keys ...string) map[string]string {
	n := f.Get(keys...)
	out := map[string]string{}
	if n == nil || n.Kind != yaml.MappingNode {
		return out
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if v := n.Content[i+1]; v.Kind == yaml.ScalarNode {
			out[n.Content[i].Value] = v.Value
		}
	}
	return out
}

// Set stores value at the key path, creating intermediate mappings.
func (f *File) Set(value any, keys ...string) error {
	if len(keys) == 0 {
		return errors.New("empty key path")
	}
	var vn yaml.Node
	if err := vn.Encode(value); err != nil {
		return err
	}

	n := f.root()
	for _, k := range keys[:len(keys)-1] {
		next := mapValue(n, k)
		if next == nil || next.Kind != yaml.MappingNode {
			next = &yaml.Node{Kind: yaml.MappingNode}
			setMapValue(n, k, next)
		}
		n = next
	}
	setMapValue(n, keys[len(keys)-1], &vn)
	return nil
}

// Delete removes the key path. It reports whether anything was removed.
func (f *File) Delete(keys ...string) bool {
	if len(keys) == 0 {
		return false
	}
	parent := f.Get(keys[:len(keys)-1]...)
	if parent == nil || parent.Kind != yaml.MappingNode {
		return false
	}
	last := keys[len(keys)-1]
	for i := 0; i+1 < len(parent.Content); i += 2 {
		if parent.Content[i].Value == last {
			parent.Content = append(parent.Content[:i], parent.Content[i+2:]...)
			return true
		}
	}
	return false
}

// Save writes the file atomically (temp file + rename), keeping it 0600
// because it lives next to credentials-adjacent settings.
func (f *File) Save() error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(f.doc); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	out := buf.Bytes()
	dir := filepath.Dir(f.path)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".config-*.yml")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	if _, err := tmp.Write(out); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(filePerm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), f.path)
}

func mapValue(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func setMapValue(m *yaml.Node, key string, v *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = v
			return
		}
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, v)
}

// LoadCurrent loads the config file in use by this invocation (as resolved
// by the root command), falling back to the default location.
func LoadCurrent() (*File, error) {
	path := viper.ConfigFileUsed()
	if path == "" {
		var err error
		if path, err = Path(""); err != nil {
			return nil, err
		}
	}
	return LoadFile(path)
}
