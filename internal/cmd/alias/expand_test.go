package alias

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testRoot() *cobra.Command {
	root := &cobra.Command{Use: "jt"}
	root.PersistentFlags().String("context", "", "")
	root.PersistentFlags().Bool("debug", false, "")
	root.AddCommand(&cobra.Command{Use: "issue", Aliases: []string{"i"}})
	return root
}

var vars = Vars{Me: "aakara", Project: "DATA", Board: "142"}

func TestExpand(t *testing.T) {
	aliases := Merge(map[string]string{
		"start":   `issue move $1 "In Progress"`,
		"blocked": `issue list -s Blocked -p{project}`,
		"sh":      `!echo {me} "$@"`,
		"mine":    "",
		"i":       "never used",
	})

	tests := []struct {
		name string
		args []string
		want *Expansion
		err  string
	}{
		{"not an alias", []string{"issue", "list"}, nil, ""},
		{"builtin alias wins", []string{"i", "list"}, nil, ""},
		{"unknown flag first", []string{"--nope", "start"}, nil, ""},
		{"global flags first", []string{"--context", "ops", "--debug", "blocked"}, &Expansion{Args: []string{"issue", "list", "-s", "Blocked", "-pDATA", "--context", "ops", "--debug"}}, ""},
		{"no args", nil, nil, ""},
		{"shell with global flags", []string{"--context=ops", "sh", "x"}, &Expansion{Shell: `echo aakara "$@"`, Args: []string{"x"}, Env: []string{"JT_CONTEXT=ops"}}, ""},
		{"positional", []string{"start", "DATA-1"}, &Expansion{Args: []string{"issue", "move", "DATA-1", "In Progress"}}, ""},
		{"positional with quote", []string{"start", "it's"}, &Expansion{Args: []string{"issue", "move", "it's", "In Progress"}}, ""},
		{"extra args appended", []string{"blocked", "--plain"}, &Expansion{Args: []string{"issue", "list", "-s", "Blocked", "-pDATA", "--plain"}}, ""},
		{"missing positional", []string{"start"}, nil, `alias "start" needs argument $1`},
		{"shell", []string{"sh", "x"}, &Expansion{Shell: `echo aakara "$@"`, Args: []string{"x"}}, ""},
		{"default alias", []string{"wip"}, &Expansion{Args: []string{"issue", "list", "-aaakara", "-sIn Progress"}}, ""},
		{"disabled default", []string{"mine"}, nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Expand(testRoot(), tt.args, aliases, vars)
			if tt.err != "" {
				assert.EqualError(t, err, tt.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestScanFlags(t *testing.T) {
	cfg, ctx := scanFlags([]string{"mine", "-c", "/tmp/a.yml", "--context=ops"})
	assert.Equal(t, "/tmp/a.yml", cfg)
	assert.Equal(t, "ops", ctx)

	cfg, ctx = scanFlags([]string{"mine", "--config=/b.yml", "--context", "data", "--", "-c", "x"})
	assert.Equal(t, "/b.yml", cfg)
	assert.Equal(t, "data", ctx)

	cfg, _ = scanFlags([]string{"-c/c.yml"})
	assert.Equal(t, "/c.yml", cfg)
}
