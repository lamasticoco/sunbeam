package tui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sunbeamlauncher/sunbeam/app"
)

type ScriptRunner struct {
	width, height int
	currentView   string

	extension app.Extension
	with      map[string]app.ScriptInputWithValue
	environ   []string

	list   *List
	detail *Detail
	form   *Form

	script app.Command
}

func NewScriptRunner(extension app.Extension, script app.Command, with map[string]app.ScriptInputWithValue) *ScriptRunner {
	mergedParams := make(map[string]app.ScriptInputWithValue)

	for _, scriptParam := range script.Inputs {
		merged := app.ScriptInputWithValue{
			ScriptInput: scriptParam,
		}

		input, ok := with[scriptParam.Name]
		if ok {
			if input.Value != nil {
				merged.Value = input.Value
			} else {
				if input.Default.Defined {
					merged.Default.Value = input.Default.Value
				}
			}
		}

		mergedParams[scriptParam.Name] = merged
	}

	return &ScriptRunner{
		extension: extension,
		script:    script,
		with:      mergedParams,
	}
}

func (c *ScriptRunner) Init() tea.Cmd {
	return c.Run()
}

type CommandOutput string

func (c ScriptRunner) ScriptCmd() tea.Msg {
	with := make(map[string]any)

	for key, param := range c.with {
		value, err := param.GetValue()
		if err != nil {
			return err
		}
		with[key] = value
	}

	commandString, err := c.script.Cmd(with)
	if err != nil {
		return err
	}

	if c.script.OnSuccess != "push-page" {
		return ExecCommandMsg{
			Exec:      commandString,
			Directory: c.extension.Root,
			Env:       c.environ,
			OnSuccess: c.script.OnSuccess,
		}
	}

	command := exec.Command("sh", "-c", commandString)
	if c.script.Page.Type == "generator" {
		command.Stdin = strings.NewReader(c.list.Query())
	}

	command.Dir = c.extension.Root
	command.Env = os.Environ()
	command.Env = append(command.Env, c.environ...)

	output, err := command.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if ok := errors.As(err, &exitErr); ok {
			return fmt.Errorf("command failed with exit code %d, error:\n%s", exitErr.ExitCode(), exitErr.Stderr)
		}
		return err
	}

	return CommandOutput(string(output))
}

func (c *ScriptRunner) CheckMissingParameters() []FormItem {
	formItems := make([]FormItem, 0)
	for _, param := range c.script.Inputs {
		input := c.with[param.Name]
		if input.Value != nil {
			continue
		}

		formItem := NewFormItem(input.ScriptInput)
		formItems = append(formItems, formItem)
	}

	return formItems
}

func (c ScriptRunner) Preferences() map[string]app.ScriptInputWithValue {
	preferences := make([]app.ScriptInput, 0, len(c.extension.Preferences)+len(c.script.Preferences))
	preferences = append(preferences, c.extension.Preferences...)
	preferences = append(preferences, c.script.Preferences...)

	preferenceMap := make(map[string]app.ScriptInputWithValue)
	for _, preference := range preferences {
		preferenceMap[preference.Name] = app.ScriptInputWithValue{
			ScriptInput: preference,
		}
	}

	return preferenceMap
}

func (c *ScriptRunner) checkPreferences() (environ []string, missing []FormItem) {
	envMap := make(map[string]struct{})
	for _, env := range os.Environ() {
		pair := strings.SplitN(env, "=", 2)
		envMap[pair[0]] = struct{}{}
	}

	for name, param := range c.Preferences() {
		if _, ok := envMap[name]; ok {
			continue
		}

		if pref, ok := keyStore.GetPreference(c.extension.Name, c.script.Name, name); ok {
			environ = append(environ, fmt.Sprintf("%s=%s", name, pref.Value))
			continue
		}

		missing = append(missing, NewFormItem(param.ScriptInput))
	}

	return environ, missing
}

func (c *ScriptRunner) Run() tea.Cmd {
	environ, missing := c.checkPreferences()
	if len(missing) > 0 {
		c.currentView = "form"
		title := fmt.Sprintf("%s · Preferences", c.extension.Title)
		c.form = NewForm("preferences", title, missing)
		c.form.SetSize(c.width, c.height)
		return c.form.Init()
	}
	c.environ = environ

	formItems := c.CheckMissingParameters()

	if len(formItems) > 0 {
		c.currentView = "form"

		title := fmt.Sprintf("%s · Params", c.extension.Title)
		c.form = NewForm("params", title, formItems)
		c.form.SetSize(c.width, c.height)
		return c.form.Init()
	}

	if c.script.OnSuccess != "push-page" {
		if c.form != nil {
			cmd := c.form.SetIsLoading(true)
			return tea.Batch(cmd, c.ScriptCmd)
		}
		return c.ScriptCmd
	}

	if c.script.Page.Type == "detail" {
		c.currentView = "detail"
		if c.detail != nil {
			cmd := c.detail.SetIsLoading(true)
			return tea.Batch(cmd, c.ScriptCmd)
		}

		c.detail = NewDetail(c.extension.Title)
		c.detail.SetSize(c.width, c.height)
		cmd := c.detail.SetIsLoading(true)
		return tea.Batch(c.ScriptCmd, cmd, c.detail.Init())
	}

	if c.script.Page.Type == "list" {
		c.currentView = "list"
		if c.list != nil {
			cmd := c.list.SetIsLoading(true)
			return tea.Batch(cmd, c.ScriptCmd)
		}
		c.list = NewList(c.extension.Title)
		if c.script.Page.IsGenerator {
			c.list.Dynamic = true
		}
		if c.script.Page.ShowPreview {
			c.list.ShowPreview = true
		}

		c.list.SetSize(c.width, c.height)

		cmd := c.list.SetIsLoading(true)
		return tea.Batch(c.ScriptCmd, c.list.Init(), cmd)
	}

	return NewErrorCmd(fmt.Errorf("unknown page type: %s", c.script.Page.Type))
}

func (c *ScriptRunner) SetSize(width, height int) {
	c.width, c.height = width, height
	switch c.currentView {
	case "list":
		c.list.SetSize(width, height)
	case "detail":
		c.detail.SetSize(width, height)
	case "form":
		c.form.SetSize(width, height)
	}
}

func (c *ScriptRunner) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch msg := msg.(type) {
	case CommandOutput:
		switch c.script.Page.Type {
		case "detail":
			var detail app.Detail
			err := json.Unmarshal([]byte(msg), &detail)
			if err != nil {
				return c, NewErrorCmd(err)
			}

			c.detail.SetIsLoading(false)
			cmd := c.detail.SetDetail(detail)
			c.SetSize(c.width, c.height)

			return c, cmd
		case "list":
			scriptItems, err := app.ParseListItems(string(msg))
			if err != nil {
				return c, NewErrorCmd(err)
			}
			listItems := make([]ListItem, len(scriptItems))

			for i, scriptItem := range scriptItems {
				if scriptItem.Id == "" {
					scriptItem.Id = strconv.Itoa(i)
				}

				for i, action := range scriptItem.Actions {
					if action.Extension == "" {
						action.Extension = c.extension.Name
						action.Dir = c.extension.Root
					}
					scriptItem.Actions[i] = action
				}

				listItems[i] = ParseScriptItem(scriptItem)
			}

			cmd := c.list.SetItems(listItems)
			c.list.SetIsLoading(false)
			return c, cmd
		}
	case SubmitMsg:
		switch msg.Name {
		case "preferences":
			preferences := make([]ScriptPreference, 0)
			for _, input := range c.extension.Preferences {
				value, ok := msg.Values[input.Name]
				if !ok {
					continue
				}
				preference := ScriptPreference{
					Name:      input.Name,
					Value:     value,
					Extension: c.extension.Name,
				}
				preferences = append(preferences, preference)
			}

			for _, input := range c.script.Preferences {
				value, ok := msg.Values[input.Name]
				if !ok {
					continue
				}
				preference := ScriptPreference{
					Name:      input.Name,
					Value:     value,
					Extension: c.extension.Name,
					Script:    c.script.Name,
				}
				preferences = append(preferences, preference)
			}

			err := keyStore.SetPreference(preferences...)
			if err != nil {
				return c, NewErrorCmd(err)
			}

			return c, c.Run()
		case "params":
			for key, value := range msg.Values {
				param, ok := c.with[key]
				if !ok {
					return c, NewErrorCmd(fmt.Errorf("unknown param: %s", key))
				}

				param.Value = value
				c.with[key] = param
			}
			return c, c.Run()
		}

	case ReloadPageMsg:
		for key, value := range msg.With {
			c.with[key] = value
		}

		return c, c.Run()
	}

	var cmd tea.Cmd
	var container Page

	switch c.currentView {
	case "list":
		container, cmd = c.list.Update(msg)
		c.list, _ = container.(*List)
	case "detail":
		container, cmd = c.detail.Update(msg)
		c.detail, _ = container.(*Detail)
	case "form":
		container, cmd = c.form.Update(msg)
		c.form, _ = container.(*Form)
	}
	return c, cmd
}

func (c *ScriptRunner) View() string {
	switch c.currentView {
	case "list":
		return c.list.View()
	case "detail":
		return c.detail.View()
	case "form":
		return c.form.View()
	default:
		return ""
	}
}
