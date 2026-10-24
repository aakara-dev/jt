# jt

A keyboard-driven Jira **Server / Data Center** client: a k9s/LazyVim-style
terminal UI plus a fast CLI. You can switch between projects, search from a
floating window, move cards across the sprint board, and generate your
standup without leaving the terminal.

jt is a hard fork of [ankitpokhrel/jira-cli](https://github.com/ankitpokhrel/jira-cli)
(MIT). Every upstream command still works under the `jt` name; see
[docs/commands.md](docs/commands.md).

```text
 DATA Sprint 42  3d left                                                                                 ━━━━──────────────────── 2/12 done

 TO DO 5                            IN PROGRESS 4                      IN REVIEW 1                        DONE 2
╭────────────────────────────────╮ ╭────────────────────────────────╮ ╭────────────────────────────────╮ ╭────────────────────────────────╮
│ ✖ DATA-2                  ⇈ AT │ │ ◼ DATA-1                  ↑ AL │ │ ◼ DATA-5                  ↓ LT │ │ ✔ DATA-3                  = GH │
│ Nightly orders job fails on    │ │ Add retry with backoff to the  │ │ Expose freshness metrics in    │ │ Document lineage for the       │
│ NULL customer ids              │ │ orders loader                  │ │ Grafana                        │ │ finance mart                   │
╰────────────────────────────────╯ ╰────────────────────────────────╯ ╰────────────────────────────────╯ ╰────────────────────────────────╯
╭────────────────────────────────╮ ╭────────────────────────────────╮                                    ╭────────────────────────────────╮
│ ↳ DATA-6                  = -- │ │ ◈ DATA-4                  = AL │                                    │ ✔ DATA-9                  ⇊ LT │
│ Add Delta OPTIMIZE to weekly   │ │ Migrate batch jobs from Oracle │                                    │ Clean up stale feature         │
│ maintenance                    │ │ to Databricks                  │                                    │ branches                       │
╰────────────────────────────────╯ ╰────────────────────────────────╯                                    ╰────────────────────────────────╯
╭────────────────────────────────╮ ╭────────────────────────────────╮
│ ◼ DATA-8                  = MH │ │ ✖ DATA-7                  ↑ AT │
│ Salt skewed join keys in       │ │ dbt incremental model          │
```

More screens, in colour: [docs/screens/index.html](docs/screens/index.html)
(or `cat docs/screens/*.ans` in a terminal).

## Install

```sh
go install github.com/aakara-dev/jt/cmd/jt@latest
```

Or build from source with `make install`. Release binaries are produced with
goreleaser (`.goreleaser.yml`).

## Set up

```sh
export JT_API_TOKEN=...      # a Personal Access Token; or use ~/.netrc / the OS keyring ("jt")
jt init                      # server, auth, first project and board
jt project add ops OPS       # save more projects (board is picked interactively)
jt                           # open the UI
```

The config lives at `~/.config/jt/config.yml`. See
[docs/config.example.yml](docs/config.example.yml) for every option. The
token never goes in the config file. `JIRA_*` environment variables from
jira-cli are still honoured as a fallback for `JT_*`.

## The UI

Run `jt` (or `jt ui`). It opens on the **My work** dashboard.

| Key | |
|---|---|
| `space` | leader menu (which-key) — every list, search mode and setting |
| `:` | command line with completion: `:board`, `:mine`, `:project ops`, `:query blocked`, `:DATA-12`, `:export md` |
| `ctrl+k` / `space /` | floating search |
| `ctrl+p` | switch project |
| `tab` | cycle home → issues → board → sprint → backlog → recent |
| `j k gg G ctrl+d ctrl+u` | move |
| `enter` / `l`, `esc` / `h` / `q` | open, back |
| `/` | fuzzy filter the current list |
| `v` / `V` | mark issues for bulk actions |
| `?` | every key for the current view |

**Issue actions** (on the selection, or on all marked issues):
- `t` transition, `a` assign, `A` assign to me
- `c` comment, `e` edit field, `E` edit description in `$EDITOR`
- `n` new issue, `w` log work (`1h 30m reviewed PR`)
- `L` link, `W` watch
- `m` move to sprint, `B` move to backlog
- `K`/`J` rank, `D` delete (asks first)
- `o` open in browser, `y`/`Y` copy key/URL, `b` copy a git branch name

**Board** (`:board`) shows the active sprint (or kanban work) in the board's
own columns:
- `h`/`l` move between columns and `j`/`k` between cards.
- `H`/`L` move a card to the neighbouring column by transitioning it.
- `[` `]` switch between parallel sprints, and `S` shows sprint stats.

**Floating search** modes, chosen by the first character or cycled with `tab`:

| Input | Searches |
|---|---|
| `retry loader` | text in the current project |
| `>status = Blocked` | raw JQL |
| `#123`, `#OPS-7` | jump to an issue |
| `@me`, `@alan` | assigned to or reported by |
| `*blocked` | saved queries and your Jira favourite filters |
| `!orders` | text across all saved projects |

Search window keys:
- `ctrl+o` opens the issue in the browser, `ctrl+y` copies its key.
- `ctrl+q` turns the results into a list view, `ctrl+s` saves the query.
- With an empty box, it shows your recently viewed issues.

Other niceties:
- **Instant lists:** they render from a local cache while refreshing.
- **Notifications:** you get a toast when a watched issue changes (`tui.watch_interval`).
- **Epic progress:** the epics list shows a completion bar per epic.
- **Export:** `:export csv|md` writes the current view.
- **Themes:** `:theme` offers tokyonight, catppuccin, gruvbox and nord.

## The CLI

```sh
jt project               # list saved projects; jt project ops / jt project - to switch
jt i ls -p ops           # any command, in another project
jt find '@me'            # same syntax as the floating search; -i opens it in the UI
jt mine                  # built-in aliases: mine wip todo cur mycur recent watching bugs
jt alias set start 'issue move $1 "In Progress"'
jt branch DATA-12 --start  # git branch feat/DATA-12-… and move the issue to In Progress
jt i v                   # on feat/DATA-12-…: views DATA-12 (key taken from the branch)
jt standup --copy        # yesterday / today / blockers as markdown
jt DATA-12               # open an issue in the UI
```

Short forms: `i` issue, `ep` epic, `sp` sprint, `b` board, `p` project,
`rel` release, and `v a c e n w wl o cp` on issue subcommands. Shell completion
covers projects, aliases and commands: `jt completion zsh`.

## Development

```sh
make test                                   # go test -race ./...
make lint                                   # golangci-lint (pinned version)
JT_SCREENS=docs/screens go test ./internal/tui -run TestScreens   # re-render screens
docker compose up -d                        # a local Jira DC to try things against
```

The TUI talks to Jira through a small `Backend` interface (`internal/tui/backend.go`).
Tests drive the whole app against a fake, and `cmd/jt/e2e_test.go` runs the
real binary against a fake Jira HTTP server.

## License

MIT. Original work © Ankit Pokhrel; see [LICENSE](LICENSE).
