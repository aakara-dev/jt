# Command reference

Inherited from [ankitpokhrel/jira-cli](https://github.com/ankitpokhrel/jira-cli) and adapted to `jt`.
Install and auth steps for jt itself are in the [README](../README.md).

## Usage
The tool currently comes with an issue, epic, and sprint explorer. The flags are [POSIX-compliant](https://www.gnu.org/software/libc/manual/html_node/Argument-Syntax.html).
You can combine available flags in any order to create a unique query. For example, the command below will give you high priority issues created this month
with status `To Do` that are assigned to you and has the label `backend`.
```sh
jt issue list -yHigh -s"To Do" --created month -lbackend -a$(jt me)
```

### Navigation
The lists are displayed in an interactive UI by default.
- Use arrow keys or `j, k, h, l` characters to navigate through the list.
- Use `g` and `G` to quickly navigate to the top and bottom respectively.
- Use `CTRL + f` to scroll through a page downwards direction.
- Use `CTRL + b` to scroll through a page in upwards direction.
- Press `v` to view selected issue details.
- Press `m` to transition the selected issue.
- Press `CTRL + r` or `F5` to refresh the issues list.
- Hit `ENTER` to open the selected issue in the browser.
- Press `c` to copy issue URL to the system clipboard. This requires `xclip` / `xsel` on Linux.
- Press `CTRL + k` to copy issue key to the system clipboard.
- In an explorer view, press `w` or `TAB` to toggle focus between the sidebar and the contents screen.
- Press `q` / `ESC` / `CTRL + c` to quit.
- Press `?` to open the help window.

### Resources
- [FAQs](https://github.com/ankitpokhrel/jira-cli/discussions/categories/faqs)
- [Introduction and Motivation](https://medium.com/@ankitpokhrel/introducing-jira-cli-the-missing-command-line-tool-for-atlassian-jira-fe44982cc1de)
- [Getting Started with JiraCLI](https://www.mslinn.com/blog/2022/08/12/jiracli.html)

## Commands
### Issue
Issues are displayed in an interactive table view by default. You can output the results in a plain view using the `--plain` flag.

#### List
The `list` command lets you search and navigate the issues. The issues are sorted by `created` field in descending order by default.

```sh
# List recent issues
$ jt issue list

# List issues created in last 7 days
$ jt issue list --created -7d

# List issues in status "To Do"
$ jt issue list -s"To Do"

# List recent issues in plain mode
$ jt issue list --plain

# List recent issues in raw JSON format
$ jt issue list --raw

# List recent issues in csv format
$ jt issue list --csv

# List issue in the same order as you see in the UI
$ jt issue list --order-by rank --reverse

# You can execute raw JQL within a given project context using `--jql/-q` option.
# For instance, the following command will list issues in the current project whose
# summary has a word cli.
$ jt issue list -q "summary ~ cli"
```

Check some more examples/use-cases below.

<details><summary>List issues that I am watching</summary>

```sh
jt issue list -w
```
</details>

<details><summary>List issues assigned to me</summary>

```sh
jt issue list -a$(jt me)
```
</details>

<details><summary>List issues assigned to a user and are reported by another user</summary>

```sh
jt issue list -a"User A" -r"User B"
```
</details>

<details><summary>List issues assigned to me, is of high priority and is open</summary>

```sh
jt issue list -a$(jt me) -yHigh -sopen
```
</details>

<details><summary>List issues assigned to no one and are created this week</summary>

```sh
jt issue list -ax --created week
```
</details>

<details><summary>List issues with resolution won't do</summary>

```sh
jt issue list -R"Won't do"
```
</details>

<details><summary>List issues whose status is not done and is created before 6 months and is assigned to someone</summary>

```sh
# Tilde (~) acts as a not operator
jt issue list -s~Done --created-before -24w -a~x
```
</details>

<details><summary>List issues created within an hour and updated in the last 30 minutes :stopwatch:</summary>

```sh
jt issue list --created -1h --updated -30m
```
</details>

<details><summary>Give me issues that are of high priority, are in progress, were created this month, and have given labels :fire:</summary>

```sh
jt issue list -yHigh -s"In Progress" --created month -lbackend -l"high-prio"
```
</details>

<details><summary>Wait, what was that ticket I opened earlier today? :tired_face:</summary>

 ```sh
 jt issue list --history
 ```
</details>

<details><summary>What was the first issue I ever reported on the current board? :thinking:</summary>

```sh
jt issue list -r$(jt me) --reverse
```
</details>

<details><summary>What was the first bug I ever fixed in the current board? :beetle:</summary>

```sh
jt issue list -a$(jt me) -tBug -sDone -rFixed --reverse
```
</details>

<details><summary>What issues did I report this week? :man_shrugging:</summary>

```sh
jt issue list -r$(jt me) --created week
```
</details>

<details><summary>Am I watching any tickets in project XYZ? :monocle_face:</summary>

```sh
jt issue list -w -pXYZ
```
</details>

#### Create
The `create` command lets you create an issue.

```sh
# Create an issue using an interactive prompt
$ jt issue create

# Pass required parameters and use --no-input option to skip prompt
$ jt issue create -tBug -s"New Bug" -yHigh -lbug -lurgent -b"Bug description" --fix-version v2.0 --no-input
```

To attach epic to an issue on creation, you can use `-P/--parent` field. We call it parent because the semantics of epic
has been changed in `next-gen` project.

```sh
# Create an issue of type Story and attach it to epic with key EPIC-42
$  jt issue create -tStory -s"Epic during creation" -PEPIC-42
```

![Create an issue](.github/assets/create.gif)

You can use a `--custom` flag to set custom fields while creating the issue. See [this post](https://github.com/ankitpokhrel/jira-cli/discussions/346) for more details.

The command supports both [GitHub-flavored](https://github.github.com/gfm/)
and [Jira-flavored](https://jira.atlassian.com/secure/WikiRendererHelpAction.jspa?section=all) Markdown for writing
description. You can load pre-defined templates using `--template` flag.

```sh
# Load description from template file
$ jt issue create --template /path/to/template.tmpl

# Get description from standard input
$ jt issue create --template -

# Or, use pipe to read input directly from standard input
$ echo "Description from stdin" | jt issue create -s"Summary" -tTask
```

![Markdown render preview](.github/assets/markdown.jpg)
> The preview above shows a Markdown template passed in JiraCLI and how it is rendered in the Jira UI.

#### Edit
The `edit` command lets you edit an issue.

```sh
$ jt issue edit ISSUE-1

# Edit issue in the configured project
$ jt issue edit ISSUE-1 -s"New Bug" -yHigh -lbug -lurgent -CBackend -b"Bug description"

# Use --no-input option to disable interactive prompt
$ jt issue edit ISSUE-1 -s"New updated summary" --no-input

# Use minus (-) to remove label, component or fixVersion
# For instance, to edit issue to
#   - remove label p2, component FE, fixVersion v1.0
#   - add label p1, component BE, fixVersion v2.0
$ jt issue edit ISSUE-1 --label -p2 --label p1 --component -FE --component BE --fix-version -v1.0 --fix-version v2.0
```

#### Assign
The `assign` command lets you assign a user to an issue.

```sh
# Assign user to an issue using an interactive prompt
$ jt issue assign

# Pass required parameters to skip prompt
$ jt issue assign ISSUE-1 "Jon Doe"

# Assign to self
$ jt issue assign ISSUE-1 $(jt me)

# Will prompt for selection if keyword suffix returns multiple entries
$ jt issue assign ISSUE-1 suffix

# Assign to default assignee
$ jt issue assign ISSUE-1 default

# Unassign
$ jt issue assign ISSUE-1 x
```

![Assign issue to a user](.github/assets/assign.gif)

#### Move/Transition
The `move` command lets you transition an issue from one state to another.

```sh
# Move an issue using an interactive prompt
$ jt issue move

# Pass required parameters to skip prompt
$ jt issue move ISSUE-1 "In Progress"
```

![Move an issue](.github/assets/move.gif)

If your workflow allows to add comment, resolution or assignee while moving an issue, you can do so as shown below.
See [this documentation](https://confluence.atlassian.com/jirakb/how-to-add-a-comment-during-a-transition-779160682.html) on how to setup your workflow to allow these fields.

```sh
# Move an issue and add comment
$ jt issue move ISSUE-1 "In Progress" --comment "Started working on it"

# Set resolution to fixed and assign to self while moving the issue
$ jt issue move ISSUE-1 Done -RFixed -a$(jt me)
```

To transition the selected issue from the TUI, press `m`.

#### View
The `view` command lets you see issue details in a terminal. Atlassian document is roughly converted to Markdown
and is nicely displayed in the terminal.

The command uses `less` as a pager by default. To set your own pager, see https://github.com/ankitpokhrel/jira-cli/discussions/569.

```sh
$ jt issue view ISSUE-1
```

![View an issue](.github/assets/view.gif)

The view screen will display linked issues and the latest comment after the description. Note that the displayed comment may
not be the latest one if you for some reason have more than 5k comments in a ticket.

```sh
# Show 5 recent comments when viewing the issue
$ jt issue view ISSUE-1 --comments 5
```

#### Link
The `link` command lets you link two issues.

```sh
# Link an issue using an interactive prompt
$ jt issue link

# Pass required parameters to skip prompt
$ jt issue link ISSUE-1 ISSUE-2 Blocks
```

##### Remote
The `remote` command lets you add a remote web link to an issue.

```sh
# Adds a remote web link using an interactive prompt
$ jt issue link remote

# Pass required parameters to skip prompt
$ jt issue link remote ISSUE-1 https://example.com "Example text"
```

#### Unlink
The `unlink` command lets you unlink two linked issues.

```sh
# Unlink issues using interactive prompt
$ jt issue unlink

# Pass required parameters to skip prompt
$ jt issue unlink ISSUE-1 ISSUE-2
```

#### Clone
The `clone` command lets you clone an issue. You can update fields like summary, priority, assignee, labels, and
components when cloning the issue. The command also allows you to replace a part of the string (case-sensitive)
in summary and description using `--replace/-H` option.

```sh
# Clone an issue
$ jt issue clone ISSUE-1

# Clone issue and modify the summary, priority and assignee
$ jt issue clone ISSUE-1 -s"Modified summary" -yHigh -a$(jt me)

# Clone issue and replace text from summary and description
$ jt issue clone ISSUE-1 -H"find me:replace with me"
```

#### Delete
The `delete` command lets you delete an issue.

```sh
# Delete an issue using an interactive prompt
$ jt issue delete

# Pass required parameters to skip prompt
$ jt issue delete ISSUE-1

# Delete task along with all of its subtasks
$ jt issue delete ISSUE-1 --cascade
```

#### Comment
The `comment` command provides a list of sub-commands to manage issue comments.

##### Add
The `add` command lets you add a comment to an issue. The command supports both [GitHub-flavored](https://github.github.com/gfm/)
and [Jira-flavored](https://jira.atlassian.com/secure/WikiRendererHelpAction.jspa?section=all) Markdown for writing
comment. You can load pre-defined templates using `--template` flag.

```sh
# Add a comment using an interactive prompt
$ jt issue comment add

# Pass required parameters to skip prompt
$ jt issue comment add ISSUE-1 "My comment body"

# Same as above but as an internal comment
$ jt issue comment add ISSUE-1 "My comment body" --internal

# Load comment from template file
$ jt issue comment add ISSUE-1 --template /path/to/template.tmpl

# Get comment from standard input
$ jt issue comment add ISSUE-1 --template -

# Or, use pipe to read input directly from standard input
$ echo "Comment from stdin" | jt issue comment add ISSUE-1
```

> [!NOTE]
> For the comment body, the positional argument always takes precedence over the `--template` flag if both of them are passed. In the
example below, the body will be picked from positional argument instead of the template.
```sh
jt issue comment add ISSUE-42 "comment body positional" --template - <<'EOF'
comment body template
EOF
```

#### Worklog
The `worklog` command provides a list of sub-commands to manage issue worklog (timelog).

##### Add
The `add` command lets you add a worklog to an issue. The command supports Markdown for worklog comments.

```sh
# Add a worklog using an interactive prompt
$ jt issue worklog add

# Pass required parameters and use --no-input to skip prompt
$ jt issue worklog add ISSUE-1 "2d 3h 30m" --no-input

# You can add a comment using --comment flag when adding a worklog
$ jt issue worklog add ISSUE-1 "10m" --comment "This is a comment" --no-input
```

### Epic
Epics are displayed in an explorer view by default. You can output the results in a table view using the `--table` flag.
When viewing epic issues, you can use all filters available for the issue command.

See [usage](#navigation) to learn more about UI interaction.

#### List
You can use all flags supported by `issue list` command here except for the issue type.

```sh
# List epics
$ jt epic list

# List epics in a table view
$ jt epic list --table

# List epics reported by me and are open
$ jt epic list -r$(jt me) -sOpen

# List issues in an epic
$ jt epic list KEY-1

# List all issue in an epic KEY-1 that is unassigned and has a high priority
$ jt epic list KEY-1 -ax -yHigh

# List high priority epics
$ jt epic list KEY-1 -yHigh

# List epic issues ordered by rank in ascending order
$ jt epic list KEY-1 --order-by rank --reverse
```

#### Create
Creating an epic is the same as creating the issue except you also need to provide an epic name.

```sh
# Create an issue using an interactive prompt
$ jt epic create

# Pass required parameters to skip prompt or use --no-input flag to skip prompt for non-mandatory params
$ jt epic create -n"Epic epic" -s"Everything" -yHigh -lbug -lurgent -b"Epic description"
```

#### Add
The `add` command allows you to add issues to the epic. You can add up to 50 issues to the epic at once.

```sh
# Add issues to the epic using an interactive prompt
$ jt epic add

# Pass required parameters to skip prompt
$ jt epic add EPIC-KEY ISSUE-1 ISSUE-2
```

#### Remove
The `remove` command allows you to remove issues from the epic. You can remove up to 50 issues from the epic at once.

```sh
# Remove issues to the epic using an interactive prompt
$ jt epic remove

# Pass required parameters to skip prompt
$ jt epic remove ISSUE-1 ISSUE-2
```

### Sprint
Sprints are displayed in an explorer view by default. You can output the results in a table view using the `--table` flag.
When viewing sprint issues, you can use all filters available for the issue command. The tool only shows 25 recent sprints.

See [usage](#navigation) to learn more about UI interaction.

#### List
You can use all flags supported by `issue list` command to filter issues in the sprint.

```sh
# List sprints in an explorer view
$ jt sprint list

# List sprints in a table view
$ jt sprint list --table

# List issues in the current active sprint
$ jt sprint list --current

# List issues in the current active sprint that are assigned to me
$ jt sprint list --current -a$(jt me)

# List issues in the previous sprint
$ jt sprint list --prev

# List issues in next planned sprint
$ jt sprint list --next

# List future and active sprints
$ jt sprint list --state future,active

# List issues in a particular sprint. You can use all flags supported by issue list command here.
# To get sprint id use `jt sprint list` or `jt sprint list --table`
$ jt sprint list SPRINT_ID

# List high priority issues in a sprint are assigned to me
$ jt sprint list SPRINT_ID -yHigh -a$(jt me)

# List sprint issues ordered by rank in ascending order
$ jt sprint list SPRINT_ID --order-by rank --reverse
```

#### Add
The `add` command allows you to add issues to the sprint. You can add up to 50 issues to the sprint at once.

```sh
# Add issues to the sprint using interactive prompt
$ jt sprint add

# Pass required parameters to skip prompt
$ jt sprint add SPRINT_ID ISSUE-1 ISSUE-2
```

### Releases

Interact with releases (project versions).  
Ensure the [feature is enabled](https://support.atlassian.com/jira-software-cloud/docs/enable-releases-and-versions/) on your instance.

#### List

```sh
# List releases for default project
$ jt release list

# List releases for specific project
$ jt release list --project 1000
$ jt release list --project KEY
```

### Other commands

<details><summary>Navigate to the project</summary>

```sh
jt open
```
</details>

<details><summary>Navigate to the issue</summary>

```sh
jt open KEY-1
```
</details>

<details><summary>List all projects you have access to</summary>

```sh
jt project list
```
</details>

<details><summary>List all boards in a project</summary>

```sh
jt board list
```
</details>

## Scripts
Often times, you may want to use the output of the command to do something cool. However, the default interactive UI might not allow you to do that.
The tool comes with the `--plain` flag that displays results in a simple layout that can then be manipulated from the shell script.

Some example scripts are listed below.

<details><summary>Tickets created per day this month</summary>

```bash
#!/usr/bin/env bash

tickets=$(jt issue list --created month --plain --columns created --no-headers | awk '{print $2}' | awk -F'-' '{print $3}' | sort -n | uniq -c)

echo "${tickets}" | while IFS=$'\t' read -r line; do
  day=$(echo "${line}" | awk '{print $2}')
  count=$(echo "${line}" | awk '{print $1}')

  printf "Day #%s: %s\n" "${day}" "${count}"
done

# Output
Day #01: 19
Day #02: 10
Day #03: 21
...
```
</details>

<details><summary>Number of tickets per sprint</summary>

```bash
#!/usr/bin/env bash

sprints=$(jt sprint list --table --plain --columns id,name --no-headers)

echo "${sprints}" | while IFS=$'\t' read -r id name; do
  count=$(jt sprint list "${id}" --plain --no-headers 2>/dev/null | wc -l)

  printf "%10s: %3d\n" "${name}" $((count))
done

# Output
Sprint 3:   55
Sprint 2:   40
Sprint 1:   30
...
```
</details>

<details><summary>Number of unique assignee per sprint</summary>

```bash
#!/usr/bin/env bash

sprints=$(jt sprint list --table --plain --columns id,name --no-headers)

echo "${sprints}" | while IFS=$'\t' read -r id name; do
  count=$(jt sprint list "${id}" --plain --columns assignee --no-headers 2>/dev/null | awk '{print $2}' | awk NF | sort -n | uniq | wc -l)

  printf "%10s: %3d\n" "${name}" $((count))
done

# Output
Sprint 3:   5
Sprint 2:   4
Sprint 1:   3
```
</details>

