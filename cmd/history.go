package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/angristan/trakt-cli/api"
	"github.com/briandowns/spinner"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/mergestat/timediff"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"
)

// historyCmd represents the history command
var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "Show your watched history",
	Long:  `Show your watched history.`,
	Run: func(cmd *cobra.Command, args []string) {
		client := api.NewAPIClient()

		s := spinner.New(spinner.CharSets[2], 100*time.Millisecond)
		s.Start()
		s.Prefix = "Loading history... "

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
			fmt.Println(err)
			return
		}

		t := table.NewWriter()
		t.SetOutputMirror(os.Stdout)
		t.AppendHeader(table.Row{
			termenv.String("Type").Bold(),
			termenv.String("Title").Bold(),
			termenv.String("Watched").Bold(),
		})
		for _, v := range resp {
			switch v.Type {
			case "movie":
				t.AppendRow([]interface{}{"🎬", v, timediff.TimeDiff(v.WatchedAt)})
			case "episode":
				t.AppendRow([]interface{}{"📺", v, timediff.TimeDiff(v.WatchedAt)})
			}
		}

		t.SetStyle(table.StyleRounded)

		s.Stop()

		t.Render()
		if pagination.ItemCount > 0 {
			fmt.Printf("Page %d out of %d, %d items in total\n", pagination.Page, pagination.PageCount, pagination.ItemCount)
		}

	},
}

func init() {
	historyCmd.Flags().Int("page", 1, "")
	historyCmd.Flags().Int("limit", 10, "")
	if filepath.Base(os.Args[0]) == "trakt-"+historyCmd.Use {
		log.Printf("using %q mode...", historyCmd.Use)
		rootCmd = historyCmd
	} else {
		rootCmd.AddCommand(historyCmd)
	}

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// historyCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// historyCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
