package root

import (
	"fmt"
	"os"
	"slices"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/aakara-dev/jt/api"
	"github.com/aakara-dev/jt/internal/cmd/board"
	"github.com/aakara-dev/jt/internal/cmd/completion"
	"github.com/aakara-dev/jt/internal/cmd/ctx"
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

var (
	config      string
	contextName string
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

		override := contextName
		if override == "" {
			override = env.Get("CONTEXT")
		}
		if err := jiraConfig.ApplyContext(viper.GetViper(), jiraConfig.ActiveContext(viper.GetViper(), override)); err != nil {
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
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			subCmd := cmd.Name()
			if !cmdRequireToken(subCmd) || cmd.Annotations[ctx.SkipTokenAnnotation] == "true" {
				return
			}

			// mTLS and cf_access don't need Jira API Token.
			if viper.GetString("auth_type") != string(jira.AuthTypeMTLS) && viper.GetString("auth_type") != string(jira.AuthTypeCFAccess) {
				checkForJiraToken(viper.GetString("server"), viper.GetString("login"))
			}

			configFile := viper.ConfigFileUsed()
			if !jiraConfig.Exists(configFile) {
				cmdutil.Failed("Missing configuration file.\nRun 'jt init' to configure the tool.")
			}
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
	cmd.PersistentFlags().StringP(
		"project", "p", "",
		fmt.Sprintf(
			"Jira project to look into (defaults to %s/%s/%s.yml)",
			configHome, jiraConfig.Dir, jiraConfig.FileName,
		),
	)
	cmd.PersistentFlags().StringVar(
		&contextName, "context", "",
		"Context to use for this command instead of the current one (or set JT_CONTEXT)",
	)
	cmd.PersistentFlags().BoolVar(&debug, "debug", false, "Turn on debug output")

	cmd.SetHelpFunc(helpFunc)

	_ = cmd.RegisterFlagCompletionFunc("context", ctx.CompleteContexts)

	_ = viper.BindPFlag("config", cmd.PersistentFlags().Lookup("config"))
	_ = viper.BindPFlag("project.key", cmd.PersistentFlags().Lookup("project"))
	_ = viper.BindPFlag("debug", cmd.PersistentFlags().Lookup("debug"))

	addChildCommands(&cmd)

	return &cmd
}

func addChildCommands(cmd *cobra.Command) {
	cmd.AddCommand(
		initCmd.NewCmdInit(),
		ctx.NewCmdCtx(),
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
