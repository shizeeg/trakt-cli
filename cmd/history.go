package cmd

import (
	"log"
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mergestat/timediff"
	"github.com/spf13/cobra"

	"github.com/shizeeg/trakt-cli/api"
)

var historyTuiCmd = &cobra.Command{
	Use:   "history",
	Short: "Show your watched history",
	Long:  `Show your watched history.`,
	Run: func(cmd *cobra.Command, args []string) {
		client := api.NewAPIClient()
		//FIXME: spinner here.
		settings, err := client.GetUserSettings()
		if err != nil {
			log.Fatalf("Failed to get user settings: %v\n", err)
		}

		page, err := cmd.Flags().GetInt("page")
		if err != nil {
			log.Fatalf("Failed to get page: %v\n", err)
		}
		limit, err := cmd.Flags().GetInt("limit")
		if err != nil {
			log.Fatalf("Failed to get limit: %v\n", err)
		}

		resp, pagination, err := client.GetUserHistory(settings.User.Ids.Slug, api.PaginationsParams{
			Page:  page,
			Limit: limit,
		})
		if err != nil {
			log.Fatal(err)
		}
		columns := []table.Column{
			{Title: "TYPE", Width: 5},
			{Title: "TITLE", Width: 50},
			{Title: "WATCHED", Width: 10},
		}
		rows := make([]table.Row, limit, 4096)
		for i, v := range resp {
			switch v.Type {
			case "movie":
				rows[i] = table.Row{"🎬", v.String(), timediff.TimeDiff(v.WatchedAt)}
			case "episode":
				rows[i] = table.Row{"📺", v.String(), timediff.TimeDiff(v.WatchedAt)}
			}

			if i >= limit {
				break
			}
		}

		t := table.New(
			table.WithColumns(columns),
			table.WithRows(rows),
			table.WithFocused(true),
		)

		s := table.DefaultStyles()
		s.Header = s.Header.
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("240")).
			BorderBottom(true).
			Bold(false)
		s.Selected = s.Selected.
			Foreground(lipgloss.Color("229")).
			Background(lipgloss.Color("57")).
			Bold(false)
		t.SetStyles(s)

		m := modelTable{t}
		if _, err := tea.NewProgram(m).Run(); err != nil {
			log.Fatal("Error running program:", err)
		}

		if pagination.ItemCount > 0 {
			tea.Printf("Page %d out of %d, %d items in total\n", pagination.Page, pagination.PageCount, pagination.ItemCount)
		}
	},
}

func init() {
	historyTuiCmd.Flags().Int("page", 1, "")
	historyTuiCmd.Flags().Int("limit", 128, "")
	if filepath.Base(os.Args[0]) == "trakt-"+historyTuiCmd.Use {
		log.Printf("using %q mode...", historyTuiCmd.Use)
		rootCmd = historyTuiCmd
	} else {
		rootCmd.AddCommand(historyTuiCmd)
	}
}

var baseStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.NormalBorder()).
	BorderForeground(lipgloss.Color("240"))

type modelTable struct {
	table table.Model
}

func (m modelTable) Init() tea.Cmd { return nil }

func (m modelTable) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.table.Columns()[0].Width = (msg.Width / 100) * 6
		m.table.Columns()[1].Width = (msg.Width / 100) * 90
		m.table.Columns()[2].Width = (msg.Width / 100) * 12
		m.table.SetHeight(msg.Height - 6)
		m.table.SetWidth(msg.Width - 2)
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if m.table.Focused() {
				m.table.Blur()
			} else {
				m.table.Focus()
			}
		case "q", "ctrl+c":
			return m, tea.Quit
		case "enter":
			return m, tea.Batch(
				tea.Printf("Let's go to %s!", m.table.SelectedRow()[1]),
			)
			// FIXME: implement dinamyc page loading...
		}
	}
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m modelTable) View() string {
	return baseStyle.Render(m.table.View()) + "\n"
}
