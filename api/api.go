package api

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/adrg/xdg"
	"github.com/clarketm/json"
	"gopkg.in/yaml.v3"
)

type APIClient struct {
	// The url of the API endpoint
	Endpoint string
	// The client for accessing the API
	Client *http.Client
	// The credentials
	Credentials Credentials
}

type Credentials struct {
	ClientID     string `yaml:"client-id"`
	ClientSecret string `yaml:"client-secret"`
	AccessToken  string `yaml:"access-token"`
	DiscordAppID string `yaml:"discord-appid"`
}

// Create a new API client for the given API version.
func NewAPIClient() APIClient {

	configFile, err := xdg.SearchConfigFile("trakt-cli/config.yaml")
	if err != nil {
		log.Fatalf("Failed to read %q file, please run `trakt auth`", configFile)
	}
	config, err := os.ReadFile(configFile)
	if err != nil {
		log.Fatal(err)
	}
	var creds Credentials
	err = yaml.Unmarshal(config, &creds)
	if err != nil {
		log.Fatalf("Failed to read %q file, please run `trakt auth`", configFile)
	}

	return APIClient{
		Endpoint: "https://api.trakt.tv",
		Client: &http.Client{
			Timeout: 120 * time.Second,
			Transport: &http.Transport{
				IdleConnTimeout: 5 * time.Second,
			},
		},
		Credentials: creds,
	}
}

type AuthDeviceCodeReq struct {
	ClientID string `json:"client_id"`
}

type AuthDeviceCodeResp struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type requestParams struct {
	method     string
	path       string
	body       interface{}
	auth       bool
	pagination PaginationsParams
}

func (c *APIClient) doRequest(params requestParams) (*http.Response, error) {
	req, err := http.NewRequest(params.method, c.Endpoint+params.path, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Accept", "application/json")

	if params.body != nil {
		req.Header.Add("Content-Type", "application/json")
		body, err := json.Marshal(params.body)
		if err != nil {
			return nil, err
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
	}

	q := req.URL.Query()
	if params.pagination.Page != 0 {
		q.Add("page", fmt.Sprintf("%d", params.pagination.Page))
	}
	if params.pagination.Limit != 0 {
		q.Add("limit", fmt.Sprintf("%d", params.pagination.Limit))
	}
	req.URL.RawQuery = q.Encode()

	if params.auth {
		req.Header.Add("trakt-api-key", c.Credentials.ClientID)
		req.Header.Add("Authorization", "Bearer "+c.Credentials.AccessToken)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *APIClient) AuthDeviceCode(req *AuthDeviceCodeReq) (*AuthDeviceCodeResp, error) {
	var resp AuthDeviceCodeResp
	httpResp, err := c.doRequest(requestParams{
		method: http.MethodPost,
		path:   "/oauth/device/code",
		body:   req,
		auth:   false,
	})
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	err = json.NewDecoder(httpResp.Body).Decode(&resp)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

type AuthDeviceTokenReq struct {
	Code         string `json:"code"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

type AuthDeviceTokenResp struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	CreatedAt    int    `json:"created_at"`
}

func (c *APIClient) AuthDeviceToken(req *AuthDeviceTokenReq) (*AuthDeviceTokenResp, error) {
	var resp AuthDeviceTokenResp
	httpResp, err := c.doRequest(requestParams{
		method: http.MethodPost,
		path:   "/oauth/device/token",
		body:   req,
		auth:   false,
	})
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode == 200 {
		err = json.NewDecoder(httpResp.Body).Decode(&resp)
		if err != nil {
			return nil, err
		}
	}

	return &resp, nil
}

type UserHistory []HistoryItem

type IDs struct {
	Trakt  int         `json:"trakt"`
	Slug   string      `json:"slug,omitempty"`
	Tvdb   interface{} `json:"tvdb"`
	Imdb   string      `json:"imdb"`
	Tmdb   int         `json:"tmdb"`
	Tvrage interface{} `json:"tvrage"`
}
type HistoryItem struct {
	ID        int64     `json:"id"`
	WatchedAt time.Time `json:"watched_at"`
	Action    string    `json:"action"`
	Type      string    `json:"type"`
	Movie     struct {
		Title string `json:"title"`
		Year  int    `json:"year"`
		Ids   IDs    `json:"ids"`
	} `json:"movie,omitempty"`
	Episode struct {
		Season int    `json:"season"`
		Number int    `json:"number"`
		Title  string `json:"title"`
		Ids    IDs    `json:"ids"`
	} `json:"episode,omitempty"`
	Show struct {
		Title string `json:"title"`
		Year  int    `json:"year"`
		Ids   IDs    `json:"ids"`
	} `json:"show,omitempty"`
}

type PaginationsParams struct {
	Page  int
	Limit int
}

type Pagination struct {
	Page      string `json:"page"`
	Limit     string `json:"limit"`
	PageCount string `json:"page_count"`
	ItemCount string `json:"item_count"`
}

func (c *APIClient) GetUserHistory(user string, params PaginationsParams) (UserHistory, Pagination, error) {
	var resp UserHistory
	httpResp, err := c.doRequest(requestParams{
		method:     http.MethodGet,
		path:       fmt.Sprintf("/users/%s/history", user),
		body:       nil,
		auth:       true,
		pagination: params,
	})
	if err != nil {
		return nil, Pagination{}, err
	}
	defer httpResp.Body.Close()

	var pagination Pagination
	if httpResp.StatusCode == 200 {
		err = json.NewDecoder(httpResp.Body).Decode(&resp)
		if err != nil {
			return nil, Pagination{}, err
		}

		pagination = Pagination{
			Page:      httpResp.Header.Get("X-Pagination-Page"),
			Limit:     httpResp.Header.Get("X-Pagination-Limit"),
			PageCount: httpResp.Header.Get("X-Pagination-Page-Count"),
			ItemCount: httpResp.Header.Get("X-Pagination-Item-Count"),
		}
	}

	return resp, pagination, nil
}

type UserSettings struct {
	User struct {
		Username string `json:"username"`
		Private  bool   `json:"private"`
		Name     string `json:"name"`
		Vip      bool   `json:"vip"`
		VipEp    bool   `json:"vip_ep"`
		Ids      struct {
			Slug string `json:"slug"`
			UUID string `json:"uuid"`
		} `json:"ids"`
		JoinedAt time.Time `json:"joined_at"`
		Location string    `json:"location"`
		About    string    `json:"about"`
		Gender   string    `json:"gender"`
		Age      int       `json:"age"`
		Images   struct {
			Avatar struct {
				Full string `json:"full"`
			} `json:"avatar"`
		} `json:"images"`
		VipOg    bool `json:"vip_og"`
		VipYears int  `json:"vip_years"`
	} `json:"user"`
	Account struct {
		Timezone   string `json:"timezone"`
		DateFormat string `json:"date_format"`
		Time24Hr   bool   `json:"time_24hr"`
		CoverImage string `json:"cover_image"`
	} `json:"account"`
	Connections struct {
		Facebook bool `json:"facebook"`
		Twitter  bool `json:"twitter"`
		Google   bool `json:"google"`
		Tumblr   bool `json:"tumblr"`
		Medium   bool `json:"medium"`
		Slack    bool `json:"slack"`
		Apple    bool `json:"apple"`
	} `json:"connections"`
	SharingText struct {
		Watching string `json:"watching"`
		Watched  string `json:"watched"`
		Rated    string `json:"rated"`
	} `json:"sharing_text"`
}

func (c *APIClient) GetUserSettings() (UserSettings, error) {
	httpResp, err := c.doRequest(requestParams{
		method: http.MethodGet,
		path:   "/users/settings",
		body:   nil,
		auth:   true,
	})
	if err != nil {
		return UserSettings{}, err
	}
	defer httpResp.Body.Close()

	var resp UserSettings
	if httpResp.StatusCode == 200 {
		err = json.NewDecoder(httpResp.Body).Decode(&resp)
		if err != nil {
			return UserSettings{}, err
		}
	}

	return resp, nil
}

type TraktMovie struct {
	Title string `json:"title,omitempty"`
	Year  int    `json:"year,omitempty"`
	Ids   IDs    `json:"ids,omitempty"`
}
type TraktShow struct {
	Title string `json:"title,omitempty"`
	Year  int    `json:"year,omitempty"`
	Ids   IDs    `json:"ids,omitempty"`
}
type TraktEpisode struct {
	Season int    `json:"season,omitempty"`
	Number int    `json:"number,omitempty"`
	Title  string `json:"title,omitempty"`
	Ids    IDs    `json:"ids,omitempty"`
	// NumberAbs             any       `json:"number_abs,omitempty"`
	Overview              string    `json:"overview,omitempty"`
	FirstAired            time.Time `json:"first_aired,omitempty"`
	UpdatedAt             time.Time `json:"updated_at,omitempty"`
	Rating                int       `json:"rating,omitempty"`
	Votes                 int       `json:"votes,omitempty"`
	CommentCount          int       `json:"comment_count,omitempty"`
	AvailableTranslations []string  `json:"available_translations,omitempty"`
	Runtime               int       `json:"runtime,omitempty"`
	EpisodeType           string    `json:"episode_type,omitempty"`
}

func (te TraktEpisode) String() string {
	return fmt.Sprintf("[%d]: %dx%02d %q\n", te.Ids.Trakt, te.Season, te.Number, te.Title)
}

func (c *APIClient) EpisodeSummary(showID string, season int, episode int) (resp TraktEpisode, err error) {
	httpResp, err := c.doRequest(requestParams{
		method: http.MethodGet,
		path:   fmt.Sprintf("/shows/%s/seasons/%d/episodes/%d", showID, season, episode),
		body:   nil,
		auth:   true,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode == 200 {
		err = json.NewDecoder(httpResp.Body).Decode(&resp)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		log.Printf("EpisodeSummary %q/%d/%d | [ERR: %d] Trakt: %q\n", showID, season, episode, httpResp.StatusCode, httpResp.Status)
	}
	return
}

func (c *APIClient) TraktQuery(query, mediaType string) (resp TraktResponse, err error) {
	httpResp, err := c.doRequest(requestParams{
		method: http.MethodGet,
		path:   fmt.Sprintf("/search/%s?fields=title&query=%s", mediaType, query),
		body:   nil,
		auth:   true,
	})
	if err != nil {
		return resp, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode == 200 {
		err = json.NewDecoder(httpResp.Body).Decode(&resp)
		if err != nil {
			return resp, err
		}
	} else {
		fmt.Printf("Query: [%q] %s\n", mediaType, query)
		log.Fatalf("TraktQuery | [ERR: %d] Trakt: %q\n", httpResp.StatusCode, httpResp.Status)
	}
	return resp, nil
}

func (c *APIClient) TraktSearch(guess Guess) (result TraktResponse, err error) {
	mediaType := guess.Type
	if guess.Type == "episode" {
		mediaType = "show"
	}
	resp, err := c.TraktQuery(guess.Title, mediaType)
	if err != nil {
		log.Fatal(err)
	}
	for _, item := range resp {
		if item.Match(guess) {
			switch item.Type {
			case "show":
				ep, err := c.EpisodeSummary(item.Show.Ids.Slug, guess.Season, guess.Episode)
				if err != nil {
					log.Fatal(err)
				}
				result = append(result,
					TraktItem{Type: "episode",
						Episode: ep,
						Show:    TraktShow{Title: item.Show.Title, Year: item.Show.Year},
					})
				// return result, nil
			case "movie":
				result = append(result, item)
			}
		}
	}
	return result, err
}

type TraktResponse []TraktItem

type TraktItem struct {
	Type     string
	Score    float64      `json:"score,omitempty"`
	Progress float64      `json:"progress,omitempty"`
	Episode  TraktEpisode `json:"episode,omitempty"`
	Show     TraktShow    `json:"show,omitempty"`
	Movie    TraktMovie   `json:"movie,omitempty"`
}

func (ti TraktItem) String() string {
	switch ti.Type {
	case "movie":
		return fmt.Sprintf("%s (%04d)", ti.Movie.Title, ti.Movie.Year)
	case "episode":
		return fmt.Sprintf("%s %dx%02d %q (%04d)",
			ti.Show.Title,
			ti.Episode.Season,
			ti.Episode.Number,
			ti.Episode.Title, ti.Show.Year)
	case "show":
		return fmt.Sprintf("%s (%04d)", ti.Show.Title, ti.Show.Year)
	}
	return fmt.Sprintf("Unknown media type: %q\n", ti.Type)
}

func (ti TraktItem) IDs() (ids IDs) {
	switch ti.Type {
	case "episode":
		ids.Trakt = ti.Episode.Ids.Trakt
		ids.Imdb = ti.Episode.Ids.Imdb
	case "movie":
		ids.Trakt = ti.Movie.Ids.Trakt
		ids.Imdb = ti.Movie.Ids.Imdb
	case "show":
		ids.Trakt = ti.Show.Ids.Trakt
		ids.Imdb = ti.Show.Ids.Imdb
	}
	return ids
}

func (ti TraktItem) Match(guess Guess) bool {
	switch guess.Type {
	case "movie":
		if guess.Year == 0 || ti.Movie.Year == guess.Year {
			return true
		}
	// case "episode":
	// 	if ti.Episode.Season == guess.Season &&
	// 		ti.Episode.Number == guess.Episode {
	// 		if guess.Year == 0 || ti.Show.Year == guess.Year {
	// 			return true
	// 		}
	// 	}
	case "episode", "show":
		if guess.Year == 0 || ti.Show.Year == guess.Year {
			return true
		}
	}
	return false
}

// func (si *TraktItem) Payload() interface{} {
// 	switch si.Type {
// 	case "episode":
// 		payload := TraktPayload{}
// 		payload.Episode.Ids = si.Episode.Ids
// 		payload.Episode.Progress = si.Progress
// 		return payload.Episode
// 	case "movie":
// 		return si.Movie
// 	}
// 	return si
// }

// type TraktPayload struct {
// 	Episode struct {
// 		Ids struct {
// 			Trakt int    `json:"trakt,omitempty"`
// 			Tvdb  int    `json:"tvdb,omitempty"`
// 			Imdb  string `json:"imdb,omitempty"`
// 			Tmdb  int    `json:"tmdb,omitempty"`
// 			// Tvrage string `json:"tvrage,omitempty"`
// 		} `json:"ids,omitempty"`
// 		Progress float64 `json:"progress,omitempty"`
// 	} `json:"episode,omitempty"`
// 	Movie struct {
// 		Ids struct {
// 			Trakt int    `json:"trakt,omitempty"`
// 			Tvdb  int    `json:"tvdb,omitempty"`
// 			Imdb  string `json:"imdb,omitempty"`
// 			Tmdb  int    `json:"tmdb,omitempty"`
// 			// Tvrage string `json:"tvrage,omitempty"`
// 		} `json:"ids,omitempty"`
// 		Progress float64 `json:"progress,omitempty"`
// 	} `json:"movie,omitempty"`
// }

type ScrobbleItem struct {
	TraktItem
	ID       int     `json:"id,omitempty"`
	Action   string  `json:"action,omitempty"`
	Progress float64 `json:"progress,omitempty"`
	Sharing  struct {
		Twitter  bool `json:"twitter,omitempty"`
		Mastodon bool `json:"mastodon,omitempty"`
		Tumblr   bool `json:"tumblr,omitempty"`
	} `json:"sharing,omitempty"`
}

func (c *APIClient) TraktScrobbleStart(item TraktItem) (ti ScrobbleItem, err error) {
	return c.traktScrobble(item, "start")
}
func (c *APIClient) TraktScrobbleStop(item TraktItem) (ti ScrobbleItem, err error) {
	return c.traktScrobble(item, "stop")
}
func (c *APIClient) TraktScrobblePause(item TraktItem) (ti ScrobbleItem, err error) {
	return c.traktScrobble(item, "pause")
}
func (c *APIClient) traktScrobble(item TraktItem, verb string) (ti ScrobbleItem, err error) {
	//DEBUG: dump what we sent to trakt to the terminal
	fmt.Println(jsonDump(item))
	//

	httpResp, err := c.doRequest(requestParams{
		method: http.MethodPost,
		path:   fmt.Sprintf("/scrobble/%s", verb),
		body:   item,
		auth:   true,
	})
	if err != nil {
		log.Fatalln(err)
		return
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode == 201 {
		err = json.NewDecoder(httpResp.Body).Decode(&ti)
		if err != nil {
			return
		}
	} else {
		log.Fatalf("traktScrobble%s | [ERR: %d] Trakt: %q\n", verb, httpResp.StatusCode, httpResp.Status)
	}
	return
}

func jsonDump(v interface{}) string {
	out, err := json.MarshalIndent(v, "", "   ")
	if err != nil {
		log.Fatalf("marshaling error: %s", err)
	}
	return string(out)
}
