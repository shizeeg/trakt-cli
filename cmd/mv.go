package cmd

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/shizeeg/trakt-cli/api"

	"github.com/spf13/cobra"
)

var mvCmd = &cobra.Command{
	Use:   "mv",
	Short: "move/rename a file according to the trakt.tv metadata.",
	Long:  `move/rename a file according to the trakt.tv metadata.`,
	Args:  cobra.MinimumNArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			//FIXME: cconsider Walk() w/ a callback
			files, err := filepath.Glob("*.mkv")
			if err != nil {
				log.Printf("can't list files: %v\n", err)
			}
			for _, f := range files {
				guess, err := api.Guessit(f)
				if err != nil {
					log.Fatal(err)
				}
				fmt.Print(guess)
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(mvCmd)
}
