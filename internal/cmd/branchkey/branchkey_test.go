package branchkey

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func testRoot() *cobra.Command {
	root := &cobra.Command{Use: "jt"}
	root.PersistentFlags().StringP("project", "p", "", "")
	issue := &cobra.Command{Use: "issue", Aliases: []string{"i"}}
	view := &cobra.Command{Use: "view", Aliases: []string{"v"}, Run: func(*cobra.Command, []string) {}}
	view.Flags().Uint("comments", 1, "")
	view.Flags().Bool("plain", false, "")
	move := &cobra.Command{Use: "move", Aliases: []string{"mv"}, Run: func(*cobra.Command, []string) {}}
	del := &cobra.Command{Use: "delete", Run: func(*cobra.Command, []string) {}}
	comment := &cobra.Command{Use: "comment", Aliases: []string{"c"}}
	add := &cobra.Command{Use: "add", Run: func(*cobra.Command, []string) {}}
	comment.AddCommand(add)
	issue.AddCommand(view, move, del, comment)
	root.AddCommand(issue)
	return root
}

func TestInject(t *testing.T) {
	branch := func() string { return "DATA-12" }
	cases := []struct {
		args, want []string
	}{
		{[]string{"issue", "view"}, []string{"issue", "view", "DATA-12"}},
		{[]string{"i", "v", "--plain"}, []string{"issue", "view", "--plain", "DATA-12"}},
		{[]string{"i", "v", "--comments", "5"}, []string{"issue", "view", "--comments", "5", "DATA-12"}},
		{[]string{"i", "v", "OPS-1"}, []string{"i", "v", "OPS-1"}},
		{[]string{"i", "v", "7"}, []string{"i", "v", "7"}},
		{[]string{"i", "mv", "Done"}, []string{"issue", "move", "DATA-12", "Done"}},
		{[]string{"-p", "ops", "i", "c", "add", "nice work"}, []string{"issue", "comment", "add", "-p", "ops", "DATA-12", "nice work"}},
		{[]string{"issue", "delete"}, []string{"issue", "delete"}},
		{[]string{"."}, []string{"ui", "DATA-12"}},
	}
	for _, c := range cases {
		got, _ := Inject(testRoot(), c.args, branch)
		assert.Equal(t, c.want, got, "%v", c.args)
	}
}

func TestInjectWithoutBranchKey(t *testing.T) {
	got, key := Inject(testRoot(), []string{"issue", "view"}, func() string { return "" })
	assert.Equal(t, []string{"issue", "view"}, got)
	assert.Equal(t, "", key)
}
