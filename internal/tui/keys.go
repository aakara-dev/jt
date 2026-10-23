package tui

// Global keys, handled by the app before the active view.
const (
	keyQuit      = "ctrl+c"
	keyBack      = "esc"
	keyBackAlt   = "q"
	keyCmdline   = ":"
	keyLeader    = "space"
	keySearch    = "ctrl+k"
	keyHelp      = "?"
	keyRefresh   = "ctrl+r"
	keyOpen      = "o"
	keyCopyKey   = "y"
	keyCopyURL   = "Y"
	keyCopyBr    = "b"
	keyNextView  = "tab"
	keyProjectSw = "ctrl+p" // quick project picker, like k9s :ctx
)

// leaderEntry is one item in the <space> which-key menu.
type leaderEntry struct {
	Key, Desc string
	Cmd       string // a ":" command line to run
	Group     string
}

// leaderMenu is the LazyVim-style <space> menu. Each entry maps to a ":"
// command, so everything reachable here is also scriptable via aliases.
var leaderMenu = []leaderEntry{
	{"/", "search", "search", "Find"},
	{"k", "jump to issue key", "search #", "Find"},
	{"j", "JQL query", "search >", "Find"},
	{"f", "saved filters", "search *", "Find"},

	{"b", "board (kanban)", "board", "Lists"},
	{"i", "issues", "issues", "Lists"},
	{"m", "assigned to me", "mine", "Lists"},
	{"s", "active sprint", "sprint", "Lists"},
	{"B", "backlog", "backlog", "Lists"},
	{"e", "epics", "epics", "Lists"},
	{"r", "recently viewed", "recent", "Lists"},
	{"w", "watching", "watching", "Lists"},
	{"u", "updated today", "updated", "Lists"},

	{"p", "switch project", "project", "Project"},
	{"-", "previous project", "project -", "Project"},
	{"t", "theme", "theme", "Project"},

	{"?", "help", "help", "App"},
	{"q", "quit", "q", "App"},
}

// globalHints are shown in help under "Global".
var globalHints = []Hint{
	{"j/k ↓/↑", "move"},
	{"gg / G", "top / bottom"},
	{"ctrl+d / ctrl+u", "half page down / up"},
	{"enter / l", "open"},
	{"esc / q / h", "back"},
	{"space", "leader menu"},
	{"ctrl+k  space /", "floating search"},
	{":", "command line"},
	{"ctrl+p", "switch project"},
	{"tab", "cycle list views"},
	{"o", "open in browser"},
	{"y / Y", "copy key / URL"},
	{"b", "copy git branch name"},
	{"ctrl+r", "refresh"},
	{"?", "help"},
	{"ctrl+c", "quit"},
}

// commandHelp documents the ":" commands.
var commandHelp = []Hint{
	{":board", "sprint / kanban board"},
	{":issues :mine :sprint :backlog", "issue lists"},
	{":epics :recent :watching :bugs", "more lists"},
	{":epic KEY", "issues in an epic"},
	{":project [name|-]  :p", "switch project"},
	{":query JQL|NAME", "run JQL or a saved query"},
	{":save NAME", "save current list's JQL"},
	{":search [text]", "floating search"},
	{":theme [name]", "change theme"},
	{":KEY-123", "open an issue"},
	{":q / :quit", "quit"},
}
