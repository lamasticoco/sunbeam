package pages

import (
	"fmt"
	"log"
	"path"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	commands "github.com/pomdtr/sunbeam/commands"
	"github.com/pomdtr/sunbeam/utils"
)

type CommandContainer struct {
	width   int
	height  int
	command commands.Command
	spinner spinner.Model
	embed   Page
}

func NewCommandContainer(command commands.Command) *CommandContainer {
	s := spinner.New()
	s.Spinner = spinner.Line
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	return &CommandContainer{command: command, spinner: s}
}

func (c *CommandContainer) headerView() string {
	line := strings.Repeat("─", c.width)
	return fmt.Sprintf("\n%s", line)
}

func (c *CommandContainer) SetSize(width, height int) {
	c.width = width
	c.height = height
	if c.embed != nil {
		c.embed.SetSize(width, height)
	}
}

func (c *CommandContainer) Init() tea.Cmd {
	return tea.Batch(c.spinner.Tick, c.fetchItems(c.command))
}

func (c CommandContainer) fetchItems(command commands.Command) tea.Cmd {
	return func() tea.Msg {
		res, err := command.Run()
		if err != nil {
			return err
		}
		return res
	}
}

func (c *CommandContainer) footerView() string {
	title := lipgloss.NewStyle().Render(c.command.Title())
	line := strings.Repeat("─", utils.Max(0, c.width-lipgloss.Width(c.command.Title())))
	return lipgloss.JoinHorizontal(lipgloss.Center, title, line)
}

func (c *CommandContainer) Update(msg tea.Msg) (Page, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEscape:
			if c.embed == nil {
				return c, PopCmd
			}
		}
	case commands.ScriptResponse:
		actionRunner := NewActionRunner(c.command)
		switch msg.Type {
		case "list":
			c.embed = NewListContainer(c.command.Title(), msg.List, actionRunner)
			c.embed.SetSize(c.width, c.height)
		case "detail":
			c.embed = NewDetailContainer(msg.Detail, actionRunner)
			c.embed.SetSize(c.width, c.height)
		case "form":
			submitAction := func(values map[string]string) tea.Cmd {
				if msg.Form.Method == "args" {
					for _, arg := range c.command.Metadatas.Arguments {
						c.command.Arguments = append(c.command.Arguments, values[arg.Placeholder])
					}
					return c.fetchItems(c.command)
				} else if msg.Form.Method == "stdin" {
					c.command.Form = values
					return c.fetchItems(c.command)
				}
				return utils.NewErrorCmd("unknown form method: %s", msg.Form.Method)
			}
			c.embed = NewFormContainer(c.command.Title(), msg.Form.Items, submitAction)
			c.embed.SetSize(c.width, c.height)
		case "action":
			cmd = NewActionRunner(c.command)(*msg.Action)
			return c, cmd
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		c.spinner, cmd = c.spinner.Update(msg)
		return c, cmd
	}

	if c.embed != nil {
		c.embed, cmd = c.embed.Update(msg)
	}

	return c, cmd
}

var titleStyle = lipgloss.NewStyle().
	Background(lipgloss.Color("62")).
	Foreground(lipgloss.Color("230")).
	Margin(0, 2).
	Padding(0, 1)

func (container *CommandContainer) View() string {
	if container.embed != nil {
		return container.embed.View()
	}

	var loadingIndicator string
	spinner := lipgloss.NewStyle().Padding(0, 2).Render(container.spinner.View())
	label := lipgloss.NewStyle().Render("Loading...")
	loadingIndicator = lipgloss.JoinHorizontal(lipgloss.Center, spinner, label)
	loadingIndicator = lipgloss.NewStyle().Padding(1, 0).Render(loadingIndicator)

	newLines := strings.Repeat("\n", utils.Max(0, container.height-lipgloss.Height(loadingIndicator)-lipgloss.Height(container.footerView())-lipgloss.Height(container.headerView())-1))

	return lipgloss.JoinVertical(lipgloss.Left, container.headerView(), loadingIndicator, newLines, container.footerView())
}

func NewActionRunner(command commands.Command) func(commands.ScriptAction) tea.Cmd {
	return func(action commands.ScriptAction) tea.Cmd {

		if action.Type != "push" {
			err := commands.RunAction(action)
			if err != nil {
				return utils.SendMsg(err)
			}

			return tea.Quit
		}

		commandDir := path.Dir(command.Url.Path)
		scriptPath := path.Join(commandDir, action.Path)
		script, err := commands.Parse(scriptPath)
		if err != nil {
			log.Fatal(err)
		}

		next := commands.Command{}
		next.Script = script
		next.Arguments = action.Args

		return NewPushCmd(NewCommandContainer(next))

	}
}