package api

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"runtime/pprof"
	"strconv"
	"time"

	"github.com/clarketm/json"

	// "encoding/json"
	"github.com/spf13/viper"
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
	ClientID     string `toml:"trakt.client-id"`
	ClientSecret string `toml:"trakt.client-secret"`
	AccessToken  string `toml:"trakt.access-token"`
	RefreshToken string `toml:"trakt.refresh-token"`
	DiscordAppID string `toml:"discord-appid"`
}

// Create a new API client for the given API version.
func NewAPIClient() APIClient {

	xdgConfDir, err := os.UserConfigDir()
	if err != nil {
		log.Fatalf("Failed to read %q, please run `trakt-cli auth`", xdgConfDir)
	}

	viper.SetConfigName("config")
	viper.SetConfigType("toml")
	viper.AddConfigPath(xdgConfDir)

	if err := viper.ReadInConfig(); err != nil {
		log.Fatal("Can't read config: ", err)
	}

	return APIClient{
		Endpoint: "https://api.trakt.tv",
		Client: &http.Client{
			Timeout: 120 * time.Second,
			Transport: &http.Transport{
				IdleConnTimeout: 5 * time.Second,
				Proxy:           http.ProxyFromEnvironment,
			},
		},
		Credentials: Credentials{
			ClientID:     viper.GetString("trakt.client-id"),
			ClientSecret: viper.GetString("trakt.client-secret"),
			AccessToken:  viper.GetString("trakt.access-token"),
			RefreshToken: viper.GetString("trakt.refresh-token"),
		},
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
	body       any
	auth       bool
	pagination PaginationsParams
}

func pathUnescape(s string) string {
	out, err := url.PathUnescape(s)
	if err != nil {
		return s
	}
	return out
}

func (c *APIClient) doRequest(params requestParams) (*http.Response, error) {
request:
	req, err := http.NewRequest(params.method, c.Endpoint+pathUnescape(params.path), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Accept", "application/json")
	req.Header.Add("trakt-api-version", "2")

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
	// The Token has expired
	if resp.StatusCode == 401 {
		log.Printf("token: %q has expired, trying to get a new one...\n", c.Credentials.AccessToken)

		rresp, err := c.RefreshToken(&AuthTokenReq{
			RefreshToken: c.Credentials.RefreshToken,
			ClientID:     c.Credentials.ClientID,
			ClientSecret: c.Credentials.ClientSecret,
			RedirectURI:  "urn:ietf:wg:oauth:2.0:oob",
			GrantType:    "refresh_token",
		})
		if err != nil {
			return nil, err
		}
		if rresp.AccessToken == "" || rresp.RefreshToken == "" {
			log.Fatalf("ERROR: %v: No access or refresh token received\n", err)
		}
		log.Printf("[INFO]: we got a new token: %q ⇒ %q\n", c.Credentials.AccessToken, rresp.AccessToken)
		c.Credentials.AccessToken = rresp.AccessToken
		c.Credentials.RefreshToken = rresp.RefreshToken
		viper.Set("trakt.access-token", rresp.AccessToken)
		viper.Set("trakt.refresh-token", rresp.RefreshToken)
		viper.WriteConfig()
		goto request

	}

	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *APIClient) RefreshToken(req *AuthTokenReq) (resp AuthTokenResp, err error) {
	httpResp, err := c.doRequest(requestParams{
		method: http.MethodPost,
		path:   "/oauth/token",
		body:   req,
		auth:   false,
	})
	if err != nil {
		return
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode == 200 {
		err = json.NewDecoder(httpResp.Body).Decode(&resp)
		if err != nil {
			return
		}
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

type AuthTokenResp struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	CreatedAt    int    `json:"created_at"`
}

type AuthTokenReq struct {
	RefreshToken string `json:"refresh_token"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RedirectURI  string `json:"redirect_uri"`
	GrantType    string `json:"grant_type"`
}

func (c *APIClient) AuthDeviceToken(req *AuthDeviceTokenReq) (resp *AuthTokenResp, err error) {
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
	return resp, nil
}

type UserHistory []HistoryItem

func (uh UserHistory) AssignRatings(ratings UserHistory) {
	// assign rating to every history item
	for i, v := range uh {
		hiID := v.IDs().Trakt
		for _, k := range ratings {
			if k.Type == v.Type && hiID == k.IDs().Trakt {
				uh[i].Rating = k.Rating
			}
		}
	}

}

type IDs struct {
	Trakt  int    `json:"trakt,omitempty"`
	Slug   string `json:"slug,omitempty"`
	Tvdb   any    `json:"tvdb,omitempty"`
	Imdb   string `json:"imdb,omitempty"`
	Tmdb   int    `json:"tmdb,omitempty"`
	Tvrage any    `json:"tvrage,omitempty"`
}

type HistoryItem struct {
	ID        int64     `json:"id,omitempty"`
	Type      string    `json:"type,omitempty"`
	Action    string    `json:"action,omitempty"`
	Rating    float64   `json:"rating,omitempty"`
	WatchedAt time.Time `json:"watched_at,omitempty"`
	RatedAt   time.Time `json:"rated_at,omitempty"`
	Movie     struct {
		Ids   IDs    `json:"ids,omitempty"`
		Title string `json:"title"`
		Year  int    `json:"year"`
	} `json:"movie,omitempty"`
	Episode struct {
		Ids    IDs    `json:"ids,omitempty"`
		Season int    `json:"season"`
		Number int    `json:"number"`
		Title  string `json:"title"`
	} `json:"episode,omitempty"`
	Show struct {
		Title string `json:"title"`
		Year  int    `json:"year"`
		Ids   IDs    `json:"ids,omitempty"`
	} `json:"show,omitempty"`
}

type PaginationsParams struct {
	Page  int
	Limit int
}

type Pagination struct {
	Page      int `json:"page"`
	Limit     int `json:"limit"`
	PageCount int `json:"page_count"`
	ItemCount int `json:"item_count"`
}

func (c *APIClient) GetHistoryWithRatings(params PaginationsParams) (resp UserHistory, pagination Pagination, err error) {
	resp, pagination, err = c.GetUserHistory("", params)
	if err != nil {
		return nil, pagination, err
	}
	// fetch all user ratings because user might request an arbitrary page from history
	//FIXME: empty params got stuck for some reason
	usrRatings, _, err := c.GetRatings(PaginationsParams{Page: 1, Limit: 128})
	if err != nil {
		// we can't get user ratings.
		// Return History as it is and report the error
		return resp, pagination, err
	}
	resp.AssignRatings(usrRatings)
	f, err := os.Create("/tmp/profile.log")
	if err != nil {
		log.Fatal(err)
	}
	pprof.StartCPUProfile(f)
	defer pprof.StopCPUProfile()

	return resp, pagination, nil
}

func (c *APIClient) GetUserHistory(user string, params PaginationsParams) (resp UserHistory, pagination Pagination, err error) {
	path := "/sync/history"
	if user != "" {
		path = fmt.Sprintf("/users/%s/history", user)

	}
	httpResp, err := c.doRequest(requestParams{
		method:     http.MethodGet,
		path:       path,
		body:       nil,
		auth:       true,
		pagination: params,
	})
	if err != nil {
		return nil, Pagination{}, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode == 200 {
		err = json.NewDecoder(httpResp.Body).Decode(&resp)
		if err != nil {
			return nil, Pagination{}, err
		}

		int_or_zero := func(s string) int {
			num, _ := strconv.Atoi(httpResp.Header.Get(s))
			return num
		}
		pagination = Pagination{
			Page:      int_or_zero("X-Pagination-Page"),
			Limit:     int_or_zero("X-Pagination-Limit"),
			PageCount: int_or_zero("X-Pagination-Page-Count"),
			ItemCount: int_or_zero("X-Pagination-Item-Count"),
		}
	}

	return resp, pagination, nil
}

type RatingResponse struct {
	Added struct {
		Episodes int `json:"episodes,omitempty"`
		Movies   int `json:"movies,omitempty"`
		Seasons  int `json:"seasons,omitempty"`
		Shows    int `json:"shows,omitempty"`
	} `json:"added,omitempty"`
	NotFound struct {
		Episodes []any `json:"episodes,omitempty"`
		Movies   []struct {
			Ids struct {
				Imdb string `json:"imdb,omitempty"`
			} `json:"ids,omitempty"`
			Rating int `json:"rating,omitempty"`
		} `json:"movies,omitempty"`
		Seasons []any `json:"seasons,omitempty"`
		Shows   []any `json:"shows,omitempty"`
	} `json:"not_found,omitempty"`
}

type UserRatings struct {
	Movies   []TraktMovie   `json:"movies,omitempty"`
	Episodes []TraktEpisode `json:"episodes,omitempty"`
}

func (c *APIClient) AddRatings(ratings UserRatings) (resp RatingResponse, err error) {
	httpResp, err := c.doRequest(requestParams{
		method: http.MethodPost,
		path:   "/sync/ratings",
		body:   ratings,
		auth:   true,
	})
	if err != nil {
		return RatingResponse{}, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode == 201 {
		err = json.NewDecoder(httpResp.Body).Decode(&resp)
		if err != nil {
			return RatingResponse{}, err
		}
	}
	return resp, nil

}

func (c *APIClient) GetRatings(params PaginationsParams) (resp UserHistory, pagination Pagination, err error) {
	httpResp, err := c.doRequest(requestParams{
		method:     http.MethodGet,
		path:       "/sync/ratings",
		body:       nil,
		auth:       true,
		pagination: params,
	})
	if err != nil {
		return nil, Pagination{}, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode == 200 {
		err = json.NewDecoder(httpResp.Body).Decode(&resp)
		if err != nil {
			return nil, Pagination{}, err
		}

		int_or_zero := func(s string) int {
			num, _ := strconv.Atoi(httpResp.Header.Get(s))
			return num
		}
		pagination = Pagination{
			Page:      int_or_zero("X-Pagination-Page"),
			Limit:     int_or_zero("X-Pagination-Limit"),
			PageCount: int_or_zero("X-Pagination-Page-Count"),
			ItemCount: int_or_zero("X-Pagination-Item-Count"),
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
	Ids     IDs       `json:"ids,omitempty"`
	Rating  float64   `json:"rating,omitempty"`
	RatedAt time.Time `json:"rated_at,omitempty"`
	Title   string    `json:"title,omitempty"`
	Year    int       `json:"year,omitempty"`
}
type TraktShow struct {
	Title string `json:"title,omitempty"`
	Year  int    `json:"year,omitempty"`
	Ids   IDs    `json:"ids,omitempty"`
}
type TraktEpisode struct {
	Ids    IDs    `json:"ids,omitempty"`
	Season int    `json:"season,omitempty"`
	Number int    `json:"number,omitempty"`
	Title  string `json:"title,omitempty"`
	// NumberAbs             any       `json:"number_abs,omitempty"`
	Overview              string    `json:"overview,omitempty"`
	FirstAired            time.Time `json:"first_aired,omitempty"`
	UpdatedAt             time.Time `json:"updated_at,omitempty"`
	RatedAt               time.Time `json:"rated_at,omitempty"`
	Rating                float64   `json:"rating,omitempty"`
	Votes                 int       `json:"votes,omitempty"`
	CommentCount          int       `json:"comment_count,omitempty"`
	AvailableTranslations []string  `json:"available_translations,omitempty"`
	Runtime               int       `json:"runtime,omitempty"`
	EpisodeType           string    `json:"episode_type,omitempty"`
}

func (te TraktEpisode) String() string {
	return fmt.Sprintf("[%d]: %dx%02d %q\n", te.Ids.Trakt, te.Season, te.Number, te.Title)
}

func (hi HistoryItem) String() string {
	switch hi.Type {
	case "episode":
		return fmt.Sprintf("%s %dx%02d %q", hi.Show.Title, hi.Episode.Season, hi.Episode.Number, hi.Episode.Title)
	case "movie":
		return fmt.Sprintf("%s (%04d)", hi.Movie.Title, hi.Movie.Year)
	}
	return "unknown media type " + hi.Type
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
	fmt.Printf("Query: [%q] %s\n", mediaType, pathUnescape(query))
	httpResp, err := c.doRequest(requestParams{
		method: http.MethodGet,
		path:   fmt.Sprintf("/search/%s/exact?fields=title&query=%s&limit=3&page=1", mediaType, query),
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
rerun:
	for _, item := range resp {
		fmt.Println(item)
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
			default:
				log.Fatalf("something weird found: %q", item)
			}
		}
	}
	// media tagged with wrong year, perhaps?
	// let's try to find something w/o considering the year
	if err == nil && len(result) == 0 {
		guess.Year = 0
		goto rerun
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

// match on type:
//
//	if movie:
//	  match on title:
//	    if matches > 1:
//	      match on year:
//	       return the 1st match
//	if season:
//	  match on season and episode.Number:
//	    match on Show.Title:
//	      if matches > 1:
//	         match on Show.Year
//	           return 1st match
func (tr TraktResponse) RankFindNormalizedFold(target Guess) TraktItem {
	return TraktItem{}
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

func (ti HistoryItem) IDs() (ids IDs) {
	switch ti.Type {
	case "episode":
		ids = ti.Episode.Ids
		ids.Slug = ti.Show.Ids.Slug
	case "movie":
		ids = ti.Movie.Ids
	case "show":
		ids = ti.Show.Ids
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
	// fmt.Println(jsonDump(item))
	//
	httpResp, err := c.doRequest(requestParams{
		method: http.MethodPost,
		path:   fmt.Sprintf("/scrobble/%s", verb),
		body:   item,
		auth:   true,
	})
	if err != nil {
		log.Fatalln(err)
		return ti, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode == 201 {
		err = json.NewDecoder(httpResp.Body).Decode(&ti)
		if err != nil {
			return
		}
		ti.Type = ti.MediaKind()
	}
	return
}

func (ti *ScrobbleItem) MediaKind() string {
	if ti.Show.Title != "" && ti.Episode.Number <= 0 {
		return "show"
	}
	if ti.Movie.Title != "" {
		return "movie"
	} else if ti.Episode.Number > 0 {
		return "episode"
	}
	return ti.Type
}

func JsonDump(v any) string {
	out, err := json.MarshalIndent(v, "", "   ")
	if err != nil {
		log.Fatalf("marshaling error: %s", err)
	}
	return string(out)
}

func JsonDumpFile(v any, filename string) {
	out, err := json.MarshalIndent(v, "", "   ")
	if err != nil {
		log.Fatalf("marshaling error: %s", err)
	}

	err = os.WriteFile(filename, out, 0644)
	if err != nil {
		log.Fatalf("file write error: %s", err)
	}
}
