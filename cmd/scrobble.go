package cmd

import (
	"fmt"
	"log"
	"time"

	"github.com/angristan/trakt-cli/api"
	"github.com/dexterlb/mpvipc"
	discord "github.com/hugolgst/rich-go/client"
	"github.com/spf13/cobra"
)

const mpvsocket = "/tmp/mpvsocket"

var discordAppID string
var now = time.Now()

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
		discordAppID = client.Credentials.DiscordAppID
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
		if pos, err := conn.Get("time-pos/full"); err == nil {
			remaining, _ := time.ParseDuration(fmt.Sprintf("%fs", pos.(float64)))
			now = time.Now().Add(-remaining)
		} else {
			log.Println(err)
		}
		events, mpvClosed := conn.NewEventListener()
		go func() {
			defer func() {
				if _, err := client.TraktScrobbleStop(currentItem); err == nil {
					if currentItem.Progress >= 80 {
						log.Printf("thanks for watching %s\n", currentItem)
					}
				}
			}()
			for !conn.IsClosed() {
				currentItem.Progress = mpvTimePos()
				isPaused, err := conn.Get("pause")
				if err != nil {
					log.Println(err)
				}
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
							return
						}
						scrobbleItem, err = client.TraktScrobbleStop(currentItem)
						if err != nil {
							log.Fatalln(err)
						}
						log.Printf("[%s] %.02f %s", scrobbleItem.Action, scrobbleItem.Progress, currentItem)
						break
					}

				default:
					time.Sleep(time.Second)
					if !isPaused.(bool) {
						if pos, err := conn.Get("time-remaining"); err == nil {
							if dur, err := conn.Get("duration/full"); err == nil {
								discordPRC(currentItem, pos.(float64), dur.(float64))
							}
						}
					}
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

func discordPRC(ti api.TraktItem, position, duration float64) {
	err := discord.Login(discordAppID)
	if err != nil {
		log.Printf("Connect to Discord? I caint! %v\n", err)
	}
	remaining, _ := time.ParseDuration(fmt.Sprintf("%fs", position))
	// start := now.Add(-time.Duration(remaining.Microseconds()))
	err = discord.SetActivity(discord.Activity{
		Details: ti.String(),
		State: fmt.Sprintf("%s / %v",
			remaining.Truncate(time.Second),
			time.Unix(int64(duration), 0).UTC().Format("15:04:05")),

		LargeImage: "largeimageid",
		LargeText:  "This is the large image :D",
		SmallImage: "smallimageid",
		SmallText:  "And this is the small image",
		// Party: &discord.Party {
		// 	ID:         "-1",
		// 	Players:    15,
		// 	MaxPlayers: 24,
		// },
		Timestamps: &discord.Timestamps{
			Start: &now,
		},
		// Buttons: []*discord.Button{
		// 	{
		// 		Label: "IMDB",
		// 		Url:   fmt.Sprintf("https://www.imdb.com/title/%s/", ti.Episode.Ids.Imdb),
		// 	},
		// },
	})

	if err != nil {
		log.Printf("[ERR]: %v\n", err)
	}

	// Discord will only show the presence if the app is running
	// Sleep for a few seconds to see the update
	// fmt.Println("Sleeping...")
	// time.Sleep(time.Second * 10)
}
