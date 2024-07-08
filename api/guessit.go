package api

import (
	"encoding/json"
	"fmt"
	"os/exec"
)

func Guessit(filename string) (guess Guess, err error) {
	cmd := exec.Command("guessit", "--json", filename)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	if err = cmd.Start(); err != nil {
		return
	}
	err = json.NewDecoder(stdout).Decode(&guess)
	cmd.Wait()
	return
}

func (g Guess) String() string {
	switch g.Type {
	case "movie":
		return fmt.Sprintf("%s (%d)\n", g.Title, g.Year)
	case "episode":
		if g.Part > 0 {
			return fmt.Sprintf("%s %dx%02d %q (Part %d)\n", g.Title, g.Season, g.Episode, g.EpisodeTitle, g.Part)
		}
		return fmt.Sprintf("%s %dx%02d %q\n", g.Title, g.Season, g.Episode, g.EpisodeTitle)
	case "show":
		if g.Year > 0 {
			return fmt.Sprintf("%s (%04d)\n", g.Title, g.Year)
		}
		return fmt.Sprintf("%s", g.Title)
	}
	return ""

}

type Guess struct {
	Title        string `json:"title"`
	Type         string `json:"type"`
	EpisodeTitle string `json:"episode_title,omitempty"`
	Container    string `json:"container,omitempty"`
	Part         int    `json:"part,omitempty"`
	Episode      int    `json:"episode,omitempty"`
	Season       int    `json:"season,omitempty"`
	Year         int    `json:"year,omitempty"`
}
