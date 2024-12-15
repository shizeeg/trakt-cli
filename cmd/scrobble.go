package cmd

import (
	"fmt"
	"log"
	"time"

	"github.com/angristan/trakt-cli/api"
	"github.com/dexterlb/mpvipc"
	"github.com/spf13/cobra"
)

const mpvsocket = "/tmp/mpvsocket"

var scrobbleCmd = &cobra.Command{
	Use:   "scrobble",
	Short: "start scrobbling to trakt.tv",
	Long:  "Start scrobbling to trakt.tv.",
	Run: func(cmd *cobra.Command, args []string) {
		// FIXME: fetch from ~/.config/mpv/mpv.conf:input-ipc-server
		conn := mpvipc.NewConnection(mpvsocket)
		log.Printf("trying to connect to %q\n", mpvsocket)
		// waiting on mpv...
		for conn.Open() != nil {
			time.Sleep(5 * time.Second)
		}

		defer conn.Close()
		client := api.NewAPIClient()
		mpvTimePos := func() float64 {
			dur, err := conn.Get("duration/full")
			if err != nil {
				log.Print(err)
			}
			pos, err := conn.Get("time-pos/full")
			if err != nil {
				log.Println(err)
			}
			percent := float64(pos.(float64) / dur.(float64) * 100)
			log.Printf("%f / %f = %.02f%%", dur, pos, percent)
			return percent
		}

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
		currentItem := api.TraktItem{}
		scrobbleItem := api.ScrobbleItem{}
		for _, item := range tresp {
			log.Printf("found %q %s\n", item.Type, item)
			currentItem = item
			if err != nil {
				log.Fatal(err)
			}
			err = conn.Set("force-media-title", item.String())
			if err != nil {
				log.Println(err)
			}
			break
		}
		tick := time.Tick(time.Minute)

		events, mpvClosed := conn.NewEventListener()
		go func() {
			defer func() {
				if _, err := client.TraktScrobbleStop(currentItem); err == nil {
					log.Printf("thanks for watching %s\n", currentItem)
				}
			}()
			for !conn.IsClosed() {
				select {
				case <-mpvClosed:
					break
				case <-tick:
					currentItem.Progress = mpvTimePos()
					isPaused, err := conn.Get("pause")
					if err != nil {
						log.Println(err)
					}
					if !isPaused.(bool) {
						scrobbleItem, err = client.TraktScrobbleStart(currentItem)
						if err != nil {
							log.Fatalln(err)
						}

						log.Printf("[%s] %.02f %s", scrobbleItem.Action, scrobbleItem.Progress, currentItem)
					} else {
						if currentItem.Progress >= 80 { // we're done, TraktScrobbleStop() returns an empty Item so we discard it.
							break
						}
						scrobbleItem, err = client.TraktScrobbleStop(currentItem)
						if err != nil {
							log.Fatalln(err)
						}
						log.Printf("[%s] %.02f %s", scrobbleItem.Action, scrobbleItem.Progress, currentItem)
						break
					}

				default:
					time.Sleep(time.Microsecond * 50)
				}
			}
		}()
		for ev := range events {
			switch ev.Name {
			case "playback-restart":
				if scrobbleItem.Action == "start" {
					currentItem.Progress = mpvTimePos()
				}
			case "end-file":
				if scrobbleItem.Action == "playing" {
					client.TraktScrobbleStop(currentItem)
				}
			default:
				log.Printf("mpv: %q\n", ev.Name)
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(scrobbleCmd)
}
