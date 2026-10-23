package tui

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/aakara-dev/jt/pkg/jira"
	"github.com/aakara-dev/jt/pkg/md"
)

// issuesChangedMsg tells views that these issues were modified so they can
// reload.
type issuesChangedMsg struct{ keys []string }

func changed(keys ...string) tea.Msg { return issuesChangedMsg{keys} }

// actionHints document the issue actions available in every view.
var actionHints = []Hint{
	{"t", "transition"},
	{"a / A", "assign / assign to me"},
	{"c", "comment"},
	{"e / E", "edit field / edit description in $EDITOR"},
	{"n", "new issue"},
	{"w", "log work (e.g. 1h 30m fixed it)"},
	{"L", "link to another issue"},
	{"W", "watch / unwatch"},
	{"m / B", "move to sprint / backlog"},
	{"D", "delete (asks first)"},
}

// actionKeys are the keys dispatched to issue actions.
var actionKeys = map[string]bool{
	"t": true, "a": true, "A": true, "c": true, "e": true, "E": true, "n": true,
	"w": true, "L": true, "W": true, "m": true, "B": true, "D": true,
}

// keys returns the issue keys.
func keys(issues []*jira.Issue) []string {
	out := make([]string, len(issues))
	for i, iss := range issues {
		out[i] = iss.Key
	}
	return out
}

func label(issues []*jira.Issue) string {
	if len(issues) == 1 {
		return issues[0].Key
	}
	return fmt.Sprintf("%d issues", len(issues))
}

// forEach runs fn on every issue and reports one combined result.
func forEach(issues []*jira.Issue, done string, fn func(*jira.Issue) error) tea.Cmd {
	return func() tea.Msg {
		var errs []error
		for _, iss := range issues {
			if err := fn(iss); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", iss.Key, err))
			}
		}
		if len(errs) > 0 {
			return tea.BatchMsg{
				func() tea.Msg { return toastMsg{text: errors.Join(errs...).Error(), err: true} },
				func() tea.Msg { return changed(keys(issues)...) },
			}
		}
		return tea.BatchMsg{
			func() tea.Msg { return toastMsg{text: done} },
			func() tea.Msg { return changed(keys(issues)...) },
		}
	}
}

// issueAction runs the action bound to key on the target issues.
func (a *App) issueAction(key string, targets []*jira.Issue) tea.Cmd {
	if len(targets) == 0 {
		return nil
	}
	first := targets[0]
	b := a.backend
	switch key {
	case "t":
		return a.transitionAction(targets)
	case "a":
		return a.assignAction(targets)
	case "A":
		me := a.sess.Login
		return forEach(targets, "assigned "+label(targets)+" to you", func(iss *jira.Issue) error {
			return b.Assign(iss.Key, me)
		})
	case "c":
		p, cmd := newTextPopup("comment on "+label(targets), "", func(text string) tea.Cmd {
			return forEach(targets, "commented on "+label(targets), func(iss *jira.Issue) error {
				return b.Comment(iss.Key, text)
			})
		})
		a.popup = p
		return cmd
	case "e":
		return a.editAction(first)
	case "E":
		return a.editDescription(first)
	case "n":
		return a.createAction()
	case "w":
		a.popup = newPrompt("log work on "+label(targets)+" (e.g. 1h 30m reviewed PR)", "", func(in string) tea.Cmd {
			spent, comment, err := parseWorklog(in)
			if err != nil {
				return toastErr(err)
			}
			return forEach(targets, "logged "+spent+" on "+label(targets), func(iss *jira.Issue) error {
				return b.Worklog(iss.Key, spent, comment)
			})
		})
		return nil
	case "L":
		return a.linkAction(first)
	case "W":
		watch := !first.Fields.Watches.IsWatching
		verb := "watching"
		if !watch {
			verb = "stopped watching"
		}
		me := a.sess.Login
		return forEach(targets, verb+" "+label(targets), func(iss *jira.Issue) error {
			return b.Watch(iss.Key, me, watch)
		})
	case "m":
		return a.sprintAction(targets)
	case "B":
		ks := keys(targets)
		return func() tea.Msg {
			if err := b.MoveToBacklog(ks); err != nil {
				return toastMsg{text: err.Error(), err: true}
			}
			return tea.BatchMsg{toast("moved " + label(targets) + " to the backlog"), func() tea.Msg { return changed(ks...) }}
		}
	case "D":
		a.popup = confirmPopup{
			title:    "delete",
			question: fmt.Sprintf("Delete %s permanently? This cannot be undone.", label(targets)),
			onYes: forEach(targets, "deleted "+label(targets), func(iss *jira.Issue) error {
				return b.Delete(iss.Key)
			}),
		}
		return nil
	}
	return nil
}

func (a *App) transitionAction(targets []*jira.Issue) tea.Cmd {
	b := a.backend
	first := targets[0]
	return func() tea.Msg {
		ts, err := b.Transitions(first.Key)
		if err != nil {
			return toastMsg{text: err.Error(), err: true}
		}
		if len(ts) == 0 {
			return toastMsg{text: "no transitions available for " + first.Key}
		}
		items := make([]pickItem, 0, len(ts))
		for _, t := range ts {
			items = append(items, pickItem{Label: t.Name, Detail: "→ " + t.To.Name, Value: t.ID.String()})
		}
		return openPopupMsg{newPicker("transition "+label(targets), items, func(it pickItem) tea.Cmd {
			return applyTransition(b, targets, it.Label)
		})}
	}
}

// applyTransition moves each issue through the transition named name. Each
// issue's own transitions are looked up, since ids differ by workflow.
func applyTransition(b Backend, targets []*jira.Issue, name string) tea.Cmd {
	return forEach(targets, label(targets)+" → "+name, func(iss *jira.Issue) error {
		ts, err := b.Transitions(iss.Key)
		if err != nil {
			return err
		}
		for _, t := range ts {
			if strings.EqualFold(t.Name, name) {
				return b.Transition(iss.Key, t)
			}
		}
		return fmt.Errorf("transition %q not available", name)
	})
}

func (a *App) assignAction(targets []*jira.Issue) tea.Cmd {
	b, project := a.backend, a.sess.Project
	fixed := []pickItem{
		{Label: "me", Detail: a.sess.Login, Value: a.sess.Login},
		{Label: "unassigned", Detail: "remove assignee", Value: jira.AssigneeNone},
	}
	p, cmd := newAsyncPicker("assign "+label(targets), fixed, func(q string) ([]pickItem, error) {
		if strings.TrimSpace(q) == "" {
			return fixed, nil
		}
		users, err := b.AssignableUsers(project, q)
		if err != nil {
			return nil, err
		}
		items := make([]pickItem, 0, len(users))
		for _, u := range users {
			items = append(items, pickItem{Label: u.DisplayName, Detail: u.Name, Value: u.Name})
		}
		return items, nil
	}, func(it pickItem) tea.Cmd {
		return forEach(targets, "assigned "+label(targets)+" to "+it.Label, func(iss *jira.Issue) error {
			return b.Assign(iss.Key, it.Value)
		})
	})
	a.popup = p
	return cmd
}

var priorities = []string{"Highest", "High", "Medium", "Low", "Lowest"}

func (a *App) editAction(iss *jira.Issue) tea.Cmd {
	b := a.backend
	items := []pickItem{
		{Label: "summary", Detail: iss.Fields.Summary, Value: "summary"},
		{Label: "priority", Detail: iss.Fields.Priority.Name, Value: "priority"},
		{Label: "labels", Detail: strings.Join(iss.Fields.Labels, ", "), Value: "labels"},
		{Label: "description", Detail: "open in $EDITOR", Value: "description"},
	}
	edit := func(req *jira.EditRequest, done string) tea.Cmd {
		return forEach([]*jira.Issue{iss}, done, func(*jira.Issue) error { return b.Edit(iss.Key, req) })
	}
	a.popup = newPicker("edit "+iss.Key, items, func(it pickItem) tea.Cmd {
		switch it.Value {
		case "summary":
			return openPopup(newPrompt("summary", iss.Fields.Summary, func(v string) tea.Cmd {
				return edit(&jira.EditRequest{Summary: v}, "updated summary")
			}))
		case "priority":
			var pi []pickItem
			for _, p := range priorities {
				pi = append(pi, pickItem{Label: p, Value: p})
			}
			return openPopup(newPicker("priority", pi, func(p pickItem) tea.Cmd {
				return edit(&jira.EditRequest{Priority: p.Value}, "priority "+p.Value)
			}).cursorTo(iss.Fields.Priority.Name))
		case "labels":
			current := iss.Fields.Labels
			return openPopup(newPrompt("labels (space or comma separated)", strings.Join(current, " "), func(v string) tea.Cmd {
				return edit(&jira.EditRequest{Labels: labelDiff(current, parseLabels(v))}, "updated labels")
			}))
		default:
			return a.editDescription(iss)
		}
	})
	return nil
}

// labelDiff expresses the change from old to new labels in the edit API's
// add / "-remove" form.
func labelDiff(old, updated []string) []string {
	has := map[string]bool{}
	for _, l := range updated {
		has[l] = true
	}
	var out []string
	for _, l := range updated {
		out = append(out, l)
	}
	for _, l := range old {
		if !has[l] {
			out = append(out, "-"+l)
		}
	}
	return out
}

// pendingEdit remembers which issue an $EDITOR session belongs to.
type pendingEdit struct {
	id  int
	key string
}

func (a *App) editDescription(iss *jira.Issue) tea.Cmd {
	b := a.backend
	key := iss.Key
	return func() tea.Msg {
		full, err := b.Issue(key)
		if err != nil {
			return toastMsg{text: err.Error(), err: true}
		}
		current, _ := full.Fields.Description.(string)
		id, cmd := editInEditor(md.FromJiraMD(current), ".md")
		return tea.BatchMsg{func() tea.Msg { return pendingEditMsg{pendingEdit{id, key}} }, cmd}
	}
}

type pendingEditMsg struct{ p pendingEdit }

// finishEdit saves a description edited in $EDITOR.
func (a *App) finishEdit(msg editorDoneMsg) tea.Cmd {
	if a.pending.id != msg.id {
		return nil
	}
	key := a.pending.key
	a.pending = pendingEdit{}
	if msg.err != nil {
		return toastErr(msg.err)
	}
	b := a.backend
	body := md.ToJiraMD(strings.TrimRight(msg.text, "\n"))
	return forEach([]*jira.Issue{{Key: key}}, "updated description of "+key, func(*jira.Issue) error {
		return b.Edit(key, &jira.EditRequest{Body: body})
	})
}

func (a *App) linkAction(iss *jira.Issue) tea.Cmd {
	b := a.backend
	return func() tea.Msg {
		types, err := b.LinkTypes()
		if err != nil {
			return toastMsg{text: err.Error(), err: true}
		}
		var items []pickItem
		for _, t := range types {
			items = append(items,
				pickItem{Label: t.Outward, Detail: t.Name, Value: "out|" + t.Name},
				pickItem{Label: t.Inward, Detail: t.Name, Value: "in|" + t.Name})
		}
		return openPopupMsg{newPicker(iss.Key+" …", items, func(it pickItem) tea.Cmd {
			dir, name, _ := strings.Cut(it.Value, "|")
			return openPopup(newPrompt(iss.Key+" "+it.Label+" (issue key)", "", func(other string) tea.Cmd {
				other = strings.ToUpper(strings.TrimSpace(other))
				if !issueKeyRe.MatchString(other) {
					return errorf("not an issue key: %s", other)
				}
				// Jira reads a link as "<outwardIssue> <outward> <inwardIssue>".
				inward, outward := other, iss.Key
				if dir == "in" {
					inward, outward = iss.Key, other
				}
				return forEach([]*jira.Issue{iss}, iss.Key+" "+it.Label+" "+other, func(*jira.Issue) error {
					return b.Link(inward, outward, name)
				})
			}))
		})}
	}
}

func (a *App) sprintAction(targets []*jira.Issue) tea.Cmd {
	if a.sess.BoardID == 0 {
		return toast("this project has no board; add one with `jt project add NAME KEY --board ...`")
	}
	b, board := a.backend, a.sess.BoardID
	return func() tea.Msg {
		sprints, err := b.Sprints(board, "active,future")
		if err != nil {
			return toastMsg{text: err.Error(), err: true}
		}
		if len(sprints) == 0 {
			return toastMsg{text: "no active or future sprints on this board"}
		}
		items := make([]pickItem, 0, len(sprints))
		for _, s := range sprints {
			items = append(items, pickItem{Label: s.Name, Detail: s.Status, Value: fmt.Sprint(s.ID)})
		}
		ks := keys(targets)
		return openPopupMsg{newPicker("move "+label(targets)+" to sprint", items, func(it pickItem) tea.Cmd {
			var id int
			_, _ = fmt.Sscan(it.Value, &id)
			return func() tea.Msg {
				if err := b.MoveToSprint(id, ks); err != nil {
					return toastMsg{text: err.Error(), err: true}
				}
				return tea.BatchMsg{toast("moved " + label(targets) + " to " + it.Label), func() tea.Msg { return changed(ks...) }}
			}
		})}
	}
}

// createAction opens the new-issue form, prefilled from the current view.
func (a *App) createAction() tea.Cmd {
	if a.sess.Project == "" {
		return toast("no project selected: switch with ctrl+p")
	}
	b, sess := a.backend, a.sess
	epic, sprintID := "", 0
	switch v := a.top().(type) {
	case *listView:
		if strings.HasPrefix(v.spec.Title, "Epic ") {
			epic = strings.TrimPrefix(v.spec.Title, "Epic ")
		}
	case *boardView:
		if v.sprint != nil {
			sprintID = v.sprint.ID
		}
	}
	sprintChoices := []string{"backlog"}
	if sprintID != 0 {
		sprintChoices = []string{"current sprint", "backlog"}
	}
	f := newForm("new issue in "+sess.Project, func(v map[string]string) tea.Cmd {
		req := &jira.CreateRequest{
			Project:        sess.Project,
			IssueType:      v["Type"],
			Summary:        v["Summary"],
			Body:           md.ToJiraMD(v["Description"]),
			Priority:       v["Priority"],
			Labels:         parseLabels(v["Labels"]),
			ParentIssueKey: strings.ToUpper(v["Epic"]),
		}
		if v["Assignee"] == "me" {
			req.Assignee = sess.Login
		}
		toSprint := v["Sprint"] == "current sprint"
		return func() tea.Msg {
			res, err := b.Create(req)
			if err != nil {
				return toastMsg{text: err.Error(), err: true}
			}
			if toSprint {
				if err := b.MoveToSprint(sprintID, []string{res.Key}); err != nil {
					return toastMsg{text: "created " + res.Key + " but sprint move failed: " + err.Error(), err: true}
				}
			}
			return tea.BatchMsg{toast("created " + res.Key), func() tea.Msg { return changed(res.Key) }}
		}
	}).
		choice("Type", issueTypeNames(false), "Story").
		text("Summary", "", true).
		choice("Priority", priorities, "Medium").
		choice("Assignee", []string{"me", "unassigned"}, "me").
		text("Labels", "", false).
		text("Epic", epic, false).
		choice("Sprint", sprintChoices, sprintChoices[0]).
		area("Description", "")
	f, cmd := f.start()
	a.popup = f
	return tea.Batch(cmd, f.setFocus(1))
}

var worklogDuration = regexp.MustCompile(`^(\d+(\.\d+)?[wdhm]\s*)+`)

// parseWorklog splits "1h 30m reviewed PR" into ("1h 30m", "reviewed PR").
func parseWorklog(in string) (spent, comment string, err error) {
	in = strings.TrimSpace(in)
	m := worklogDuration.FindString(in)
	if m == "" {
		return "", "", fmt.Errorf("start with a duration like 2h, 1h 30m or 1d")
	}
	return strings.TrimSpace(m), strings.TrimSpace(in[len(m):]), nil
}
