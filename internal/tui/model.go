package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"sesame/internal/domain"
)

type Model struct {
	auth domain.AuthContext
}

func NewModel(auth domain.AuthContext) Model {
	return Model{auth: auth}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m Model) View() string {
	title := lipgloss.NewStyle().Bold(true).Render("Sesame")
	return fmt.Sprintf("%s\n\nAuth: %s\nRegion: %s\n\nTUI scaffold is ready. Press q to quit.\n",
		title,
		m.auth.Mode,
		m.auth.Region,
	)
}
