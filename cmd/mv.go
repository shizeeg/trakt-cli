package cmd

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/shizeeg/trakt-cli/api"

	"github.com/spf13/cobra"
)

var mvCmd = &cobra.Command{
	Use:   "mv",
	Short: "move/rename a file according to the trakt.tv metadata.",
	Long:  `move/rename a file according to the trakt.tv metadata.`,
	Args:  cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		guess, err := api.Guessit(args[0])
		if err != nil {
			log.Fatal(err)
		}

		client := api.NewAPIClient()
		resp, err := client.TraktSearch(guess)
		if err != nil {
			log.Fatalf("[ERR]: Trakt: %v\n", err)
		}
		for _, item := range resp {
			// switch item.Type {
			// default:
			// 	fmt.Printf("Unknown type %q: %v\n", item.Type, item)
			// case "episode":
			// 	fmt.Printf("%s %dx%02d %q\n", item.Show.Title, item.Episode.Season, item.Episode.Number, item.Episode.Title)
			// case "show":
			// 	fmt.Printf("%s (%04d)\n", item.Show.Title, item.Show.Year)
			// case "movie":
			// 	fmt.Printf("%s (%04d)\n", item.Movie.Title, item.Movie.Year)

			// }
			out := args[1]
			out = strings.ReplaceAll(out, "<container>", guess.Container)
			if item.Type == "movie" {
				out = strings.ReplaceAll(out, "<title>", item.Movie.Title)
				out = strings.ReplaceAll(out, "<year>", fmt.Sprintf("%04d", item.Movie.Year))
				fmt.Println(filepath.Clean(out))
			} else if item.Type == "episode" {
				out = strings.ReplaceAll(out, "<title>", item.Show.Title)
				out = strings.ReplaceAll(out, "<year>", fmt.Sprintf("%04d", item.Show.Year))
				out = strings.ReplaceAll(out, "<episodeTitle>", item.Episode.Title)
				out = strings.ReplaceAll(out, "<season>", fmt.Sprintf("%d", item.Episode.Season))
				out = strings.ReplaceAll(out, "<episode>", fmt.Sprintf("%02d", item.Episode.Number))
				fmt.Println(filepath.Clean(out))
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(mvCmd)
}
