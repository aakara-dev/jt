package alias

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/google/shlex"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	jtConfig "github.com/aakara-dev/jt/internal/config"
	"github.com/aakara-dev/jt/pkg/env"
)

// Defaults ship with jt and can be overridden (or disabled with an empty
// value) under `aliases:` in the config.
var Defaults = map[string]string{
	"mine":     `issue list -a{me} -s~Done`,
	"wip":      `issue list -a{me} -s"In Progress"`,
	"todo":     `issue list -a{me} -s"To Do" --order-by priority`,
	"cur":      `sprint list --current`,
	"mycur":    `sprint list --current -a{me}`,
	"recent":   `issue list --history`,
	"watching": `issue list --watching`,
	"bugs":     `issue list -tBug -s~Done --order-by priority`,
}

var positional = regexp.MustCompile(`\$(\d+)`)

// Expansion is the result of resolving an alias.
type Expansion struct {
	// Args replaces os.Args[1:] for a regular alias.
	Args []string
	// Shell is set for "!" aliases: run it with sh -c, Args as "$@".
	Shell string
	// Env carries global flags given before a shell alias (JT_CONTEXT,
	// JT_CONFIG_FILE) so nested jt calls in the script honour them.
	Env []string
}

// Vars supplies {me}, {project} and {board} substitutions.
type Vars struct {
	Me      string
	Project string
	Board   string
}

// VarsFromFile reads substitution values for the active context.
func VarsFromFile(f *jtConfig.File, contextOverride string) Vars {
	v := Vars{
		Me:      f.GetString("login"),
		Project: f.GetString("project", "key"),
		Board:   f.GetString("board", "id"),
	}
	ctx := contextOverride
	if ctx == "" {
		ctx = f.GetString(jtConfig.KeyCurrentContext)
	}
	if ctx != "" {
		if p := f.GetString(jtConfig.KeyContexts, ctx, "project", "key"); p != "" {
			v.Project = p
		}
		v.Board = f.GetString(jtConfig.KeyContexts, ctx, "board", "id")
	}
	return v
}

// Merge returns defaults overlaid with user aliases. An empty user value
// removes the default.
func Merge(user map[string]string) map[string]string {
	out := make(map[string]string, len(Defaults)+len(user))
	for k, v := range Defaults {
		out[k] = v
	}
	for k, v := range user {
		if strings.TrimSpace(v) == "" {
			delete(out, k)
			continue
		}
		out[k] = v
	}
	return out
}

// Names returns the alias names sorted.
func Names(aliases map[string]string) []string {
	out := make([]string, 0, len(aliases))
	for k := range aliases {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// IsBuiltin reports whether name resolves to a real command (or its alias),
// which always wins over a user alias.
func IsBuiltin(root *cobra.Command, name string) bool {
	if name == "help" || strings.HasPrefix(name, "__") {
		return true
	}
	for _, c := range root.Commands() {
		if c.Name() == name || c.HasAlias(name) {
			return true
		}
	}
	return false
}

// Expand resolves args (os.Args[1:]) against aliases. It returns nil when
// args[0] is not an alias.
func Expand(root *cobra.Command, args []string, aliases map[string]string, vars Vars) (*Expansion, error) {
	lead, args := splitGlobalFlags(root, args)
	if len(args) == 0 || IsBuiltin(root, args[0]) {
		return nil, nil
	}
	body, ok := aliases[args[0]]
	if !ok {
		return nil, nil
	}
	body = substitute(body, vars)

	if shell, ok := strings.CutPrefix(body, "!"); ok {
		var envs []string
		cfg, ctx := scanFlags(lead)
		if cfg != "" {
			envs = append(envs, env.Prefix+"CONFIG_FILE="+cfg)
		}
		if ctx != "" {
			envs = append(envs, env.Prefix+"CONTEXT="+ctx)
		}
		return &Expansion{Shell: shell, Args: args[1:], Env: envs}, nil
	}
	rest := append(lead, args[1:]...)

	used := map[int]bool{}
	var subErr error
	body = positional.ReplaceAllStringFunc(body, func(m string) string {
		n, _ := strconv.Atoi(m[1:])
		if n < 1 || n > len(rest) {
			subErr = fmt.Errorf("alias %q needs argument %s", args[0], m)
			return m
		}
		used[n] = true
		return shellQuote(rest[n-1])
	})
	if subErr != nil {
		return nil, subErr
	}

	expanded, err := shlex.Split(body)
	if err != nil {
		return nil, fmt.Errorf("alias %q: %w", args[0], err)
	}
	for i, a := range rest {
		if !used[i+1] {
			expanded = append(expanded, a)
		}
	}
	return &Expansion{Args: expanded}, nil
}

func substitute(body string, v Vars) string {
	return strings.NewReplacer(
		"{me}", v.Me,
		"{project}", v.Project,
		"{board}", v.Board,
	).Replace(body)
}

// splitGlobalFlags peels root persistent flags (e.g. `--context ops`) off
// the front of args so `jt --context ops mine` still finds the alias. The
// flags are re-appended after the expansion, where cobra accepts them too.
func splitGlobalFlags(root *cobra.Command, args []string) (lead, rest []string) {
	pf := root.PersistentFlags()
	i := 0
	for i < len(args) && strings.HasPrefix(args[i], "-") && args[i] != "--" {
		a := args[i]
		name, _, hasValue := strings.Cut(strings.TrimLeft(a, "-"), "=")
		var flag *pflag.Flag
		if strings.HasPrefix(a, "--") {
			flag = pf.Lookup(name)
		} else if len(name) > 0 {
			flag = pf.ShorthandLookup(name[:1])
			hasValue = hasValue || len(name) > 1
		}
		if flag == nil {
			return nil, args
		}
		lead = append(lead, a)
		i++
		if flag.NoOptDefVal == "" && !hasValue && i < len(args) {
			lead = append(lead, args[i])
			i++
		}
	}
	return lead, args[i:]
}

// shellQuote quotes s so shlex.Split returns it as one argument.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// Resolve loads aliases from the config selected by args (-c/--config,
// JT_CONFIG_FILE or the default) and expands args[0] if it is an alias.
// A missing or unreadable config only disables user aliases.
func Resolve(root *cobra.Command, args []string) (*Expansion, error) {
	cfgPath, ctxName := scanFlags(args)
	if ctxName == "" {
		ctxName = env.Get("CONTEXT")
	}

	user := map[string]string{}
	var vars Vars
	if path, err := jtConfig.Path(cfgPath); err == nil {
		if f, err := jtConfig.LoadFile(path); err == nil {
			user = f.StringMap(jtConfig.KeyAliases)
			vars = VarsFromFile(f, ctxName)
		}
	}
	return Expand(root, args, Merge(user), vars)
}

// scanFlags finds --config/-c and --context anywhere in args without a
// full parse; aliases are resolved before cobra sees the command line.
func scanFlags(args []string) (cfg, context string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		next := func() string {
			if i+1 < len(args) {
				i++
				return args[i]
			}
			return ""
		}
		switch {
		case a == "--":
			return cfg, context
		case a == "-c" || a == "--config":
			cfg = next()
		case strings.HasPrefix(a, "--config="):
			cfg = strings.TrimPrefix(a, "--config=")
		case strings.HasPrefix(a, "-c") && len(a) > 2 && !strings.HasPrefix(a, "--"):
			cfg = a[2:]
		case a == "--context":
			context = next()
		case strings.HasPrefix(a, "--context="):
			context = strings.TrimPrefix(a, "--context=")
		}
	}
	return cfg, context
}
