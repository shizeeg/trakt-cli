package cmd

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mergestat/timediff"
	"github.com/spf13/cobra"

	"github.com/shizeeg/trakt-cli/api"
)

func icon(item api.HistoryItem) string {
	switch item.Type {
	case "movie":
		return "🎬"
	case "episode":
		return "📺"
	}
	return "?"
}

func hearts(rating int) string {
	if rating > 10 {
		rating = 10
	}
	if rating < 0 {
		rating = 0
	}
	return strings.Repeat("\u2665", rating) + strings.Repeat("\u2661", 10-rating)
}

func toRatings(items []api.HistoryItem) (resp api.UserRatings) {
	for _, it := range items {
		switch it.Type {
		case "movie":
			resp.Movies = append(resp.Movies, api.TraktMovie{
				Rating: it.Rating,
				Title:  it.Movie.Title,
				Year:   it.Movie.Year,
				Ids:    it.Movie.Ids,
			})
		case "episode":
			resp.Episodes = append(resp.Episodes, api.TraktEpisode{
				Rating: it.Rating,
				Season: it.Episode.Season,
				Number: it.Episode.Number,
				Ids:    it.Episode.Ids,
			})
		case "season":
		case "show":
		}
	}
	return resp
}

var historyTuiCmd = &cobra.Command{
	Use:   "history",
	Short: "Show your watched history",
	Long:  `Show your watched history.`,
	Run: func(cmd *cobra.Command, args []string) {
		client := api.NewAPIClient()
		// FIXME: spinner here.

		page, err := cmd.Flags().GetInt("page")
		if err != nil {
			log.Fatalf("Failed to get page: %v\n", err)
		}
		limit, err := cmd.Flags().GetInt("limit")
		if err != nil {
			log.Fatalf("Failed to get limit: %v\n", err)
		}

		resp, pagination, err := client.GetHistoryWithRatings(api.PaginationsParams{
			Page:  page,
			Limit: limit,
		})
		if err != nil {
			log.Fatal(err)
		}

		columns := []table.Column{
			{Title: "RATING", Width: 12},
			{Title: "TITLE", Width: 50},
			{Title: "WATCHED", Width: 14},
		}
		rows := make([]table.Row, pagination.Limit, pagination.ItemCount)
		for i, v := range resp {
			rows[i] = table.Row{"  " + hearts(v.Rating), icon(v) + " " + v.String(), timediff.TimeDiff(v.WatchedAt)}

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

		m := modelTable{client: &client, table: t, history: resp, pagination: pagination, ratings: make(map[int]api.HistoryItem)}
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
	historyTuiCmd.Flags().Int("limit", 64, "")
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
	client     *api.APIClient
	table      table.Model
	history    api.UserHistory
	ratings    map[int]api.HistoryItem
	pagination api.Pagination
}

func (m modelTable) Init() tea.Cmd { return nil }

func (m modelTable) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// leave rating column width hard coded for now
		// m.table.Columns()[0].Width = (msg.Width / 100) * 12
		// m.table.Columns()[1].Width = (msg.Width / 100) * 50
		// m.table.Columns()[2].Width = (msg.Width / 100) * 14
		m.table.SetHeight(msg.Height - 8)
		// m.table.SetWidth(msg.Width - 8)
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
			hitem := m.history[m.table.Cursor()]
			var url string
			if hitem.Type == "movie" { // FIXME: should I move the code to api/api.go?
				url = fmt.Sprintf("https://trakt.tv/movies/%s", hitem.IDs().Slug)
			} else {
				url = fmt.Sprintf("https://trakt.tv/shows/%s/seasons/%d/episodes/%d",
					hitem.IDs().Slug, hitem.Episode.Season, hitem.Episode.Number)
			}
			openURL(url)
			return m, tea.Batch(
				tea.Printf("trying to browse: %q...\n", url),
			)
			// FIXME: implement dynamic page loading...
		case "left", "h", "-":
			v := m.history[m.table.Cursor()]
			if v.Rating <= 0 {
				return m, cmd
			}
			v.Rating -= 1
			m.history[m.table.Cursor()] = v
			m.table.Rows()[m.table.Cursor()] = table.Row{fmt.Sprintf("%X ", v.Rating) + hearts(v.Rating), icon(v) + " " + v.String(), timediff.TimeDiff(v.WatchedAt)}
			m.ratings[v.IDs().Trakt] = v
			m.table.UpdateViewport()
		case "right", "l", "+":
			v := m.history[m.table.Cursor()]
			if v.Rating >= 10 {
				return m, cmd
			}
			v.Rating += 1
			m.history[m.table.Cursor()] = v
			m.table.Rows()[m.table.Cursor()] = table.Row{fmt.Sprintf("%X ", v.Rating) + hearts(v.Rating), icon(v) + " " + v.String(), timediff.TimeDiff(v.WatchedAt)}
			m.ratings[v.IDs().Trakt] = v
			m.table.UpdateViewport()
		case "S": // sync ratings with traktTV
			var rItems []api.HistoryItem
			for _, hi := range m.history {
				if hi.IDs().Trakt != 0 {
					r := m.ratings[hi.IDs().Trakt]
					if r.Type != "" {
						rItems = append(rItems, r)
					}
				}
			}
			rPayload := toRatings(rItems)
			_, err := m.client.AddRatings(rPayload)
			if err != nil {
				log.Printf("WARNING %v: %q\n", err, "unable to sync data to TraktTV")
			}

		}
	}
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m modelTable) View() string {
	return baseStyle.Render(m.table.View()) + "\n"
}

// opens url in the default browser
func openURL(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	case "darwin": // macOS
		cmd = "open"
		args = []string{url}
	default: // Linux, FreeBSD, and the rest..
		cmd = "xdg-open"
		args = []string{url}
	}

	return exec.Command(cmd, args...).Start()
}
