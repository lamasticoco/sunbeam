package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sunbeamlauncher/sunbeam/app"
)

type Action struct {
	Cmd      tea.Cmd
	Shortcut string
	Title    string
}

func (a Action) Binding() key.Binding {
	prettyKey := strings.ReplaceAll(a.Shortcut, "ctrl", "⌃")
	prettyKey = strings.ReplaceAll(prettyKey, "alt", "⌥")
	prettyKey = strings.ReplaceAll(prettyKey, "shift", "⇧")
	prettyKey = strings.ReplaceAll(prettyKey, "cmd", "⌘")
	prettyKey = strings.ReplaceAll(prettyKey, "enter", "↩")

	return key.NewBinding(key.WithKeys(a.Shortcut), key.WithHelp(prettyKey, a.Title))
}

func NewCopyTextCmd(text string) tea.Cmd {
	return func() tea.Msg {
		return CopyTextMsg{
			Text: text,
		}
	}
}

type CopyTextMsg struct {
	Text string
}

func NewOpenUrlCmd(Url string) tea.Cmd {
	return func() tea.Msg {
		return OpenUrlMsg{
			Url: Url,
		}
	}
}

type OpenUrlMsg struct {
	Url string
}

func NewReloadPageCmd(with map[string]app.ScriptInputWithValue) tea.Cmd {
	return func() tea.Msg {
		return ReloadPageMsg{
			With: with,
		}
	}
}

type ReloadPageMsg struct {
	With map[string]app.ScriptInputWithValue
}

func NewRunScriptCmd(extension string, script string, with map[string]app.ScriptInputWithValue) tea.Cmd {
	return func() tea.Msg {
		return RunScriptMsg{
			Extension: extension,
			Script:    script,
			With:      with,
		}
	}
}

type RunScriptMsg struct {
	Extension string
	Script    string
	With      map[string]app.ScriptInputWithValue
	OnSuccess string
}

func (msg RunScriptMsg) OnSuccessCmd() tea.Cmd {
	switch msg.OnSuccess {
	case "exit":
		return tea.Quit
	case "reload-page":
		return tea.Sequence(PopCmd, NewReloadPageCmd(nil))
	default:
		return tea.Quit
	}
}

func NewExecCmd(command string) tea.Cmd {
	return func() tea.Msg {
		return ExecCommandMsg{
			Exec: command,
		}
	}
}

type ExecCommandMsg struct {
	Exec      string
	Directory string
	OnSuccess string
	Env       []string
}

func (msg ExecCommandMsg) OnSuccessMsg(output string) tea.Msg {
	switch msg.OnSuccess {
	case "reload-page":
		return ReloadPageMsg{}
	case "copy-text":
		return CopyTextMsg{
			Text: output,
		}
	case "open-url":
		return OpenUrlMsg{
			Url: strings.TrimSpace(output),
		}
	case "open-path":
		return OpenUrlMsg{
			Url: fmt.Sprintf("file://%s", strings.TrimSpace(output)),
		}
	default:
		return fmt.Errorf("unknown on-success action: %s", msg.OnSuccess)
	}
}

func NewEditCmd(path string) tea.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	return func() tea.Msg {
		return ExecCommandMsg{
			Exec: fmt.Sprintf("%s %s", editor, path),
		}
	}
}

func NewAction(scriptAction app.ScriptAction) Action {
	var cmd tea.Cmd
	switch scriptAction.Type {
	case "copy-text":
		if scriptAction.Title == "" {
			scriptAction.Title = "Copy to Clipboard"
		}
		cmd = NewCopyTextCmd(scriptAction.Text)
	case "reload-page":
		if scriptAction.Title == "" {
			scriptAction.Title = "Reload Script"
		}
		cmd = NewReloadPageCmd(scriptAction.With)
	case "run-command":
		cmd = func() tea.Msg {
			return RunScriptMsg{
				Extension: scriptAction.Extension,
				Script:    scriptAction.Script,
				With:      scriptAction.With,
				OnSuccess: scriptAction.OnSuccess,
			}
		}
	case "open-path":
		if scriptAction.Title == "" {
			scriptAction.Title = "Open"
		}
		cmd = NewOpenUrlCmd(fmt.Sprintf("file://%s", scriptAction.Path))
	case "open-url":
		if scriptAction.Title == "" {
			scriptAction.Title = "Open in Browser"
		}
		cmd = NewOpenUrlCmd(scriptAction.Url)
	case "edit":
		if scriptAction.Title == "" {
			scriptAction.Title = "Edit File"
		}
		cmd = NewEditCmd(scriptAction.Path)
	default:
		scriptAction.Title = "Unknown"
		cmd = NewErrorCmd(fmt.Errorf("unknown action type: %s", scriptAction.Type))
	}

	return Action{
		Cmd:      cmd,
		Title:    scriptAction.Title,
		Shortcut: scriptAction.Shortcut,
	}
}

type ActionList struct {
	header Header
	filter Filter
	footer Footer

	actions []Action
}

func NewActionList() ActionList {
	filter := NewFilter()
	filter.DrawLines = true

	header := NewHeader()
	footer := NewFooter("Actions")
	footer.SetBindings(
		key.NewBinding(key.WithKeys("tab"), key.WithHelp("↩", "Confirm")),
		key.NewBinding(key.WithKeys("tab"), key.WithHelp("⎋", "Hide Actions")),
	)

	return ActionList{
		header: header,
		filter: filter,
		footer: footer,
	}
}

func (al *ActionList) SetSize(w, h int) {
	availableHeight := h - lipgloss.Height(al.header.View()) - lipgloss.Height(al.footer.View())

	al.filter.SetSize(w, availableHeight)
	al.footer.Width = w
	al.header.Width = w
}

func (al *ActionList) SetTitle(title string) {
	al.footer.title = title
}

func (al *ActionList) SetActions(actions ...Action) {
	al.actions = actions
	filterItems := make([]FilterItem, len(actions))
	for i, action := range actions {
		filterItems[i] = ListItem{
			Title:    action.Title,
			Subtitle: action.Shortcut,
			Actions:  []Action{action},
		}
	}
	al.filter.SetItems(filterItems)
	al.filter.FilterItems(al.header.Value())
}

func (al ActionList) Update(msg tea.Msg) (ActionList, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "shift+tab":
			if !al.Focused() {
				return al, al.Focus()
			} else if al.Focused() {
				if msg.String() == "tab" {
					al.filter.CursorDown()
				} else {
					al.filter.CursorUp()
				}
			}
		case "esc":
			if !al.Focused() {
				return al, nil
			}

			if al.header.input.Value() != "" {
				al.Clear()
				return al, nil
			} else {
				al.Blur()
			}

			return al, nil

		case "enter":
			selectedItem := al.filter.Selection()
			if selectedItem == nil {
				return al, nil
			}
			listItem, _ := selectedItem.(ListItem)
			al.Clear()
			al.Blur()
			return al, listItem.Actions[0].Cmd
		}

		for _, action := range al.actions {
			if key.Matches(msg, action.Binding()) {
				al.Clear()
				al.Blur()
				return al, action.Cmd
			}
		}
	}

	var cmds []tea.Cmd
	var cmd tea.Cmd

	if !al.Focused() {
		return al, nil
	}

	header, cmd := al.header.Update(msg)
	cmds = append(cmds, cmd)
	if header.Value() != al.header.Value() {
		al.filter.FilterItems(header.Value())
	}
	al.header = header

	al.filter, cmd = al.filter.Update(msg)
	cmds = append(cmds, cmd)

	return al, tea.Batch(cmds...)
}

func (al ActionList) Focused() bool {
	return al.header.input.Focused()

}

func (al *ActionList) Focus() tea.Cmd {
	return al.header.Focus()
}

func (al *ActionList) Clear() {
	al.header.input.SetValue("")
	al.filter.FilterItems("")
}

func (al *ActionList) Blur() {
	al.header.input.Blur()
}

func (al ActionList) View() string {
	return lipgloss.JoinVertical(
		lipgloss.Left,
		al.header.View(),
		al.filter.View(),
		al.footer.View(),
	)
}
