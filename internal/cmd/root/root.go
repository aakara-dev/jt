package root

import (
	"fmt"
	"os"
	"regexp"
	"slices"

	"golang.org/x/term"

	"github.com/aakara-dev/jt/internal/cmd/ui"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/aakara-dev/jt/api"
	"github.com/aakara-dev/jt/internal/cmd/alias"
	"github.com/aakara-dev/jt/internal/cmd/board"
	"github.com/aakara-dev/jt/internal/cmd/completion"
	"github.com/aakara-dev/jt/internal/cmd/epic"
	initCmd "github.com/aakara-dev/jt/internal/cmd/init"
	"github.com/aakara-dev/jt/internal/cmd/issue"
	"github.com/aakara-dev/jt/internal/cmd/man"
	"github.com/aakara-dev/jt/internal/cmd/me"
	"github.com/aakara-dev/jt/internal/cmd/open"
	"github.com/aakara-dev/jt/internal/cmd/project"
	"github.com/aakara-dev/jt/internal/cmd/release"
	"github.com/aakara-dev/jt/internal/cmd/serverinfo"
	"github.com/aakara-dev/jt/internal/cmd/sprint"
	"github.com/aakara-dev/jt/internal/cmd/version"
	"github.com/aakara-dev/jt/internal/cmdutil"
	jiraConfig "github.com/aakara-dev/jt/internal/config"
	"github.com/aakara-dev/jt/pkg/env"
	"github.com/aakara-dev/jt/pkg/jira"
)

const (
	jiraCLIHelpLink  = "https://github.com/aakara-dev/jt#getting-started"
	jiraAPITokenLink = "https://id.atlassian.com/manage-profile/security/api-tokens"
)

var issueKeyArg = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]+-\d+$`)

var (
	config      string
	projectFlag string
	debug       bool
)

func init() {
	cobra.OnInitialize(func() {
		if config != "" {
			// 1. Command line flag has the highest priority
			viper.SetConfigFile(config)
		} else if configFile := env.Get("CONFIG_FILE"); configFile != "" {
			// 2. Environment variable has second priority
			viper.SetConfigFile(configFile)
		} else {
			// 3. Default location has the lowest priority
			home, err := cmdutil.GetConfigHome()
			if err != nil {
				cmdutil.Failed("Error: %s", err)
				return
			}

			viper.AddConfigPath(fmt.Sprintf("%s/%s", home, jiraConfig.Dir))
			viper.SetConfigName(jiraConfig.FileName)
			viper.SetConfigType(jiraConfig.FileType)
		}

		viper.AutomaticEnv()
		viper.SetEnvPrefix("jt")

		if err := viper.ReadInConfig(); err != nil {
			return
		}
		if debug {
			fmt.Printf("Using config file: %s\n", viper.ConfigFileUsed())
		}

		if err := applyProject(); err != nil {
			cmdutil.Failed("Error: %s", err)
		}
	})
}

// NewCmdRoot is a root command.
func NewCmdRoot() *cobra.Command {
	cmd := cobra.Command{
		Use:   "jt <command> <subcommand>",
		Short: "Interactive Jira CLI",
		Long:  "Interactive Jira command line.",

		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// `jt` opens the TUI; `jt DATA-12` opens an issue in it.
			if len(args) == 1 && issueKeyArg.MatchString(args[0]) {
				requireSetup()
				return ui.Run(args)
			}
			if len(args) > 0 {
				return fmt.Errorf("unknown command %q for \"jt\"\nRun 'jt --help' for usage", args[0])
			}
			if !term.IsTerminal(int(os.Stdout.Fd())) {
				return cmd.Help()
			}
			requireSetup()
			return ui.Run(nil)
		},
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			subCmd := cmd.Name()
			if !cmdRequireToken(subCmd) || cmd.Annotations[cmdutil.SkipTokenAnnotation] == "true" {
				return
			}
			requireSetup()
		},
	}

	configHome, err := cmdutil.GetConfigHome()
	if err != nil {
		cmdutil.Failed("Error: %s", err)
	}

	cmd.PersistentFlags().StringVarP(
		&config, "config", "c", "",
		fmt.Sprintf("Config file (default is %s/%s/%s.yml, can be overridden with JT_CONFIG_FILE env var)", configHome, jiraConfig.Dir, jiraConfig.FileName),
	)
	cmd.PersistentFlags().StringVarP(
		&projectFlag, "project", "p", "",
		"Saved project name or Jira project key to use instead of the current project (or set JT_PROJECT)",
	)
	cmd.PersistentFlags().BoolVar(&debug, "debug", false, "Turn on debug output")

	cmd.SetHelpFunc(helpFunc)

	_ = cmd.RegisterFlagCompletionFunc("project", project.CompleteProjects)

	_ = viper.BindPFlag("config", cmd.PersistentFlags().Lookup("config"))
	_ = viper.BindPFlag("project.key", cmd.PersistentFlags().Lookup("project"))
	_ = viper.BindPFlag("debug", cmd.PersistentFlags().Lookup("debug"))

	addChildCommands(&cmd)

	return &cmd
}

func addChildCommands(cmd *cobra.Command) {
	cmd.AddCommand(
		initCmd.NewCmdInit(),
		alias.NewCmdAlias(),
		ui.NewCmdUI(),
		issue.NewCmdIssue(),
		epic.NewCmdEpic(),
		sprint.NewCmdSprint(),
		board.NewCmdBoard(),
		project.NewCmdProject(),
		open.NewCmdOpen(),
		me.NewCmdMe(),
		serverinfo.NewCmdServerInfo(),
		completion.NewCmdCompletion(),
		version.NewCmdVersion(),
		release.NewCmdRelease(),
		man.NewCmdMan(),
	)
}

// applyProject overlays the selected saved project onto the config. The
// selector is -p/--project or JT_PROJECT; it may name a saved project or a
// Jira key. A key that matches no saved project is used as-is with the
// top-level defaults, like upstream jira-cli.
func applyProject() error {
	selector := projectFlag
	if selector == "" {
		selector = env.Get("PROJECT")
	}
	f, err := jiraConfig.LoadFile(viper.ConfigFileUsed())
	if err != nil {
		return nil //nolint:nilerr // no readable config: nothing to overlay
	}
	if selector == "" {
		return jiraConfig.ApplyProject(viper.GetViper(), f.GetString(jiraConfig.KeyCurrentProject))
	}
	name := f.Resolve(selector)
	if name == "" {
		viper.Set("project.key", selector)
		return nil
	}
	if err := jiraConfig.ApplyProject(viper.GetViper(), name); err != nil {
		return err
	}
	// The flag is bound to project.key; replace the saved name with its key.
	viper.Set("project.key", f.GetString(jiraConfig.KeyProjects, name, "project", "key"))
	return nil
}

// requireSetup exits with guidance if there is no config file or API token.
func requireSetup() {
	// mTLS and cf_access don't need Jira API Token.
	if viper.GetString("auth_type") != string(jira.AuthTypeMTLS) && viper.GetString("auth_type") != string(jira.AuthTypeCFAccess) {
		checkForJiraToken(viper.GetString("server"), viper.GetString("login"))
	}

	configFile := viper.ConfigFileUsed()
	if !jiraConfig.Exists(configFile) {
		cmdutil.Failed("Missing configuration file.\nRun 'jt init' to configure the tool.")
	}
}

func cmdRequireToken(cmd string) bool {
	allowList := []string{
		"init",
		"help",
		"jt",
		"version",
		"completion",
		"__complete", "__completeNoDesc", // Subcommand name during autocompletion call.
		"man",
	}
	return !slices.Contains(allowList, cmd)
}

func checkForJiraToken(server string, login string) {
	if api.GetAPIToken(jira.Config{
		Server: server,
		Login:  login,
	}) != "" {
		return
	}

	msg := fmt.Sprintf(`The tool needs a Jira API token to function.

For cloud server: you can generate the token using this link: %s
For local server: you can use the password you use to log in to Jira for basic auth or get a token from your Jira profile for PAT.

After generating the token, you can either:
  - Export API token to your shell as a JT_API_TOKEN env variable
  - Or, you can use a .netrc file to define required machine details

Once you are done with the above steps, run 'jt init' to generate the config if you haven't already.

For more details, see: %s
`, jiraAPITokenLink, jiraCLIHelpLink)

	cmdutil.Warn(msg)
	os.Exit(1)
}
