package cmd

import (
	"fmt"
	"log"

	"github.com/angristan/trakt-cli/api"
	"github.com/dexterlb/mpvipc"
	"github.com/spf13/cobra"
)

var scrobbleCmd = &cobra.Command{
	Use:   "scrobble",
	Short: "start scrobbling to trakt.tv",
	Long:  "Start scrobbling to trakt.tv.",
	Run: func(cmd *cobra.Command, args []string) {
		// FIXME: fetch from ~/.config/mpv/mpv.conf:input-ipc-server
		conn := mpvipc.NewConnection("/tmp/mpvsocket")
		err := conn.Open()
		if err != nil {
			log.Fatal(err)
		}
		defer conn.Close()
		client := api.NewAPIClient()

		path, err := conn.Get("path")
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("current file playing: %s", path)
		guess, err := api.Guessit(fmt.Sprint(path))
		if err != nil {
			log.Fatal(err)
		}
		tresp, err := client.TraktSearch(guess)
		if err != nil {
			log.Fatal(err)
		}
		for _, item := range tresp {
			log.Printf("found: %s\n", item)
			err = conn.Set("force-media-title", item.String())
			if err != nil {
				log.Println(err)
			}
			break
		}
		events, stopListening := conn.NewEventListener()
		// close when connection dissapeares
		go func() {
			conn.WaitUntilClosed()
			stopListening <- struct{}{}
		}()
		for ev := range events {
			log.Printf("mpv: %q\n", ev.Name)
		}
	},
}

func init() {
	rootCmd.AddCommand(scrobbleCmd)
}
