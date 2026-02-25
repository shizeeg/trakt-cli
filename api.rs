use reqwest::{Client, Method, Response};
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::fs;
use std::path::PathBuf;
use std::time::Duration;
use anyhow::{anyhow, Context, Result};

const API_ENDPOINT: &str = "https://api.trakt.tv";
const API_VERSION: &str = "2";

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Credentials {
    #[serde(rename = "trakt.client-id")]
    pub client_id: String,
    #[serde(rename = "trakt.client-secret")]
    pub client_secret: String,
    #[serde(rename = "trakt.access-token")]
    pub access_token: String,
    #[serde(rename = "trakt.refresh-token")]
    pub refresh_token: String,
    #[serde(rename = "discord-appid")]
    pub discord_app_id: Option<String>,
}

#[derive(Debug)]
pub struct APIClient {
    /// The URL of the API endpoint
    pub endpoint: String,
    /// The HTTP client for accessing the API
    pub client: Client,
    /// The credentials
    pub credentials: Credentials,
    /// Path to config file for saving updated tokens
    config_path: Option<PathBuf>,
}

impl APIClient {
    /// Create a new API client for the Trakt API
    pub fn new() -> Result<Self> {
        let config_dir = dirs::config_dir()
            .ok_or_else(|| anyhow!("Failed to get config directory, please run auth first"))?;
        
        let config_path = config_dir.join("config.toml");
        
        let config_content = fs::read_to_string(&config_path)
            .context("Can't read config file")?;
        
        let credentials: Credentials = toml::from_str(&config_content)
            .context("Failed to parse config file")?;
        
        let client = Client::builder()
            .timeout(Duration::from_secs(120))
            .pool_idle_timeout(Duration::from_secs(5))
            .build()?;
        
        Ok(APIClient {
            endpoint: API_ENDPOINT.to_string(),
            client,
            credentials,
            config_path: Some(config_path),
        })
    }
    
    /// Create API client with custom credentials (useful for testing)
    pub fn with_credentials(credentials: Credentials) -> Result<Self> {
        let client = Client::builder()
            .timeout(Duration::from_secs(120))
            .pool_idle_timeout(Duration::from_secs(5))
            .build()?;
        
        Ok(APIClient {
            endpoint: API_ENDPOINT.to_string(),
            client,
            credentials,
            config_path: None,
        })
    }
    
    async fn do_request(&mut self, params: RequestParams) -> Result<Response> {
        let url = format!("{}{}", self.endpoint, params.path);
        let mut request = self.client.request(params.method, &url);
        
        // Add headers
        request = request
            .header("Accept", "application/json")
            .header("trakt-api-version", API_VERSION);
        
        // Add body if present
        if let Some(body) = params.body {
            request = request
                .header("Content-Type", "application/json")
                .json(&body);
        }
        
        // Add pagination parameters
        let mut query_params = HashMap::new();
        if params.pagination.page != 0 {
            query_params.insert("page", params.pagination.page.to_string());
        }
        if params.pagination.limit != 0 {
            query_params.insert("limit", params.pagination.limit.to_string());
        }
        if !query_params.is_empty() {
            request = request.query(&query_params);
        }
        
        // Add authentication if required
        if params.auth {
            request = request
                .header("trakt-api-key", &self.credentials.client_id)
                .header("Authorization", format!("Bearer {}", self.credentials.access_token));
        }
        
        let response = request.send().await?;
        
        // Handle token expiration
        if response.status() == 401 {
            eprintln!("token: {:?} has expired, trying to get a new one...", self.credentials.access_token);
            
            let refresh_req = AuthTokenReq {
                refresh_token: self.credentials.refresh_token.clone(),
                client_id: self.credentials.client_id.clone(),
                client_secret: self.credentials.client_secret.clone(),
                redirect_uri: "urn:ietf:wg:oauth:2.0:oob".to_string(),
                grant_type: "refresh_token".to_string(),
            };
            
            let new_tokens = self.refresh_token(&refresh_req).await?;
            
            if new_tokens.access_token.is_empty() || new_tokens.refresh_token.is_empty() {
                return Err(anyhow!("No access or refresh token received"));
            }
            
            eprintln!("[INFO]: we got a new token: {:?} ⇒ {:?}", 
                     self.credentials.access_token, new_tokens.access_token);
            
            self.credentials.access_token = new_tokens.access_token.clone();
            self.credentials.refresh_token = new_tokens.refresh_token.clone();
            
            // Save updated tokens to config
            if let Some(config_path) = &self.config_path {
                if let Ok(mut config_content) = fs::read_to_string(config_path) {
                    // Simple TOML update (could use a proper TOML library for editing)
                    config_content = config_content.replace(
                        &format!("access-token = \"{}\"", self.credentials.access_token),
                        &format!("access-token = \"{}\"", new_tokens.access_token)
                    );
                    config_content = config_content.replace(
                        &format!("refresh-token = \"{}\"", self.credentials.refresh_token),
                        &format!("refresh-token = \"{}\"", new_tokens.refresh_token)
                    );
                    let _ = fs::write(config_path, config_content);
                }
            }
            
            // Retry the request with new token
            return self.do_request(params).await;
        }
        
        Ok(response)
    }
    
    pub async fn refresh_token(&mut self, req: &AuthTokenReq) -> Result<AuthTokenResp> {
        let response = self.do_request(RequestParams {
            method: Method::POST,
            path: "/oauth/token".to_string(),
            body: Some(serde_json::to_value(req)?),
            auth: false,
            pagination: PaginationParams::default(),
        }).await?;
        
        if response.status() == 200 {
            Ok(response.json().await?)
        } else {
            Err(anyhow!("Failed to refresh token: {}", response.status()))
        }
    }
    
    pub async fn auth_device_code(&mut self, req: &AuthDeviceCodeReq) -> Result<AuthDeviceCodeResp> {
        let response = self.do_request(RequestParams {
            method: Method::POST,
            path: "/oauth/device/code".to_string(),
            body: Some(serde_json::to_value(req)?),
            auth: false,
            pagination: PaginationParams::default(),
        }).await?;
        
        Ok(response.json().await?)
    }
    
    pub async fn auth_device_token(&mut self, req: &AuthDeviceTokenReq) -> Result<Option<AuthTokenResp>> {
        let response = self.do_request(RequestParams {
            method: Method::POST,
            path: "/oauth/device/token".to_string(),
            body: Some(serde_json::to_value(req)?),
            auth: false,
            pagination: PaginationParams::default(),
        }).await?;
        
        if response.status() == 200 {
            Ok(Some(response.json().await?))
        } else {
            Ok(None)
        }
    }
    
    pub async fn get_user_history(&mut self, user: &str, params: PaginationParams) -> Result<(UserHistory, Pagination)> {
        let path = if user.is_empty() {
            "/sync/history".to_string()
        } else {
            format!("/users/{}/history", user)
        };
        
        let response = self.do_request(RequestParams {
            method: Method::GET,
            path,
            body: None,
            auth: true,
            pagination: params,
        }).await?;
        
        let pagination = parse_pagination_headers(response.headers());
        
        if response.status() == 200 {
            let history: UserHistory = response.json().await?;
            Ok((history, pagination))
        } else {
            Err(anyhow!("Failed to get user history: {}", response.status()))
        }
    }
    
    pub async fn get_ratings(&mut self, params: PaginationParams) -> Result<(UserHistory, Pagination)> {
        let response = self.do_request(RequestParams {
            method: Method::GET,
            path: "/sync/ratings".to_string(),
            body: None,
            auth: true,
            pagination: params,
        }).await?;
        
        let pagination = parse_pagination_headers(response.headers());
        
        if response.status() == 200 {
            let history: UserHistory = response.json().await?;
            Ok((history, pagination))
        } else {
            Err(anyhow!("Failed to get ratings: {}", response.status()))
        }
    }
    
    pub async fn get_history_with_ratings(&mut self, params: PaginationParams) -> Result<(UserHistory, Pagination)> {
        let (mut history, pagination) = self.get_user_history("", params).await?;
        
        // Fetch all user ratings
        let (ratings, _) = self.get_ratings(PaginationParams { page: 1, limit: 128 }).await?;
        
        history.assign_ratings(&ratings);
        
        Ok((history, pagination))
    }
    
    pub async fn add_ratings(&mut self, ratings: &UserRatings) -> Result<RatingResponse> {
        let response = self.do_request(RequestParams {
            method: Method::POST,
            path: "/sync/ratings".to_string(),
            body: Some(serde_json::to_value(ratings)?),
            auth: true,
            pagination: PaginationParams::default(),
        }).await?;
        
        if response.status() == 201 {
            Ok(response.json().await?)
        } else {
            Err(anyhow!("Failed to add ratings: {}", response.status()))
        }
    }
    
    pub async fn get_user_settings(&mut self) -> Result<UserSettings> {
        let response = self.do_request(RequestParams {
            method: Method::GET,
            path: "/users/settings".to_string(),
            body: None,
            auth: true,
            pagination: PaginationParams::default(),
        }).await?;
        
        if response.status() == 200 {
            Ok(response.json().await?)
        } else {
            Err(anyhow!("Failed to get user settings: {}", response.status()))
        }
    }
    
    pub async fn episode_summary(&mut self, show_id: &str, season: i32, episode: i32) -> Result<TraktEpisode> {
        let response = self.do_request(RequestParams {
            method: Method::GET,
            path: format!("/shows/{}/seasons/{}/episodes/{}", show_id, season, episode),
            body: None,
            auth: true,
            pagination: PaginationParams::default(),
        }).await?;
        
        if response.status() == 200 {
            Ok(response.json().await?)
        } else {
            eprintln!("EpisodeSummary {:?}/{}/{} | [ERR: {}] Trakt: {:?}", 
                     show_id, season, episode, response.status(), response.status());
            Err(anyhow!("Failed to get episode summary: {}", response.status()))
        }
    }
    
    pub async fn trakt_query(&mut self, query: &str, media_type: &str) -> Result<TraktResponse> {
        let response = self.do_request(RequestParams {
            method: Method::GET,
            path: format!("/search/{}?fields=title&query={}", media_type, query),
            body: None,
            auth: true,
            pagination: PaginationParams::default(),
        }).await?;
        
        if response.status() == 200 {
            Ok(response.json().await?)
        } else {
            eprintln!("Query: [{:?}] {}", media_type, query);
            Err(anyhow!("TraktQuery | [ERR: {}] Trakt: {:?}", response.status(), response.status()))
        }
    }
    
    pub async fn trakt_search(&mut self, guess: &Guess) -> Result<TraktResponse> {
        let media_type = if guess.media_type == "episode" {
            "show"
        } else {
            &guess.media_type
        };
        
        let mut resp = self.trakt_query(&guess.title, media_type).await?;
        let mut result = TraktResponse::default();
        let mut modified_guess = guess.clone();
        
        loop {
            for item in &resp {
                if item.matches(&modified_guess) {
                    match item.item_type.as_str() {
                        "show" => {
                            let ep = self.episode_summary(
                                &item.show.as_ref().unwrap().ids.slug,
                                modified_guess.season,
                                modified_guess.episode
                            ).await?;
                            result.push(TraktItem {
                                item_type: "episode".to_string(),
                                score: item.score,
                                progress: item.progress,
                                episode: Some(ep),
                                show: item.show.clone(),
                                movie: None,
                            });
                        }
                        "movie" => {
                            result.push(item.clone());
                        }
                        _ => {
                            return Err(anyhow!("something weird found: {:?}", item));
                        }
                    }
                }
            }
            
            // Media tagged with wrong year? Try without year
            if result.is_empty() && modified_guess.year != 0 {
                modified_guess.year = 0;
                continue;
            }
            
            break;
        }
        
        Ok(result)
    }
    
    pub async fn trakt_scrobble_start(&mut self, item: &TraktItem) -> Result<ScrobbleItem> {
        self.trakt_scrobble(item, "start").await
    }
    
    pub async fn trakt_scrobble_stop(&mut self, item: &TraktItem) -> Result<ScrobbleItem> {
        self.trakt_scrobble(item, "stop").await
    }
    
    pub async fn trakt_scrobble_pause(&mut self, item: &TraktItem) -> Result<ScrobbleItem> {
        self.trakt_scrobble(item, "pause").await
    }
    
    async fn trakt_scrobble(&mut self, item: &TraktItem, verb: &str) -> Result<ScrobbleItem> {
        let response = self.do_request(RequestParams {
            method: Method::POST,
            path: format!("/scrobble/{}", verb),
            body: Some(serde_json::to_value(item)?),
            auth: true,
            pagination: PaginationParams::default(),
        }).await?;
        
        if response.status() == 201 {
            let mut scrobble: ScrobbleItem = response.json().await?;
            scrobble.item_type = scrobble.media_kind();
            Ok(scrobble)
        } else {
            Err(anyhow!("Failed to scrobble: {}", response.status()))
        }
    }
}

#[derive(Debug)]
struct RequestParams {
    method: Method,
    path: String,
    body: Option<serde_json::Value>,
    auth: bool,
    pagination: PaginationParams,
}

#[derive(Debug, Clone, Copy, Default, Serialize, Deserialize)]
pub struct PaginationParams {
    pub page: i32,
    pub limit: i32,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct Pagination {
    pub page: i32,
    pub limit: i32,
    pub page_count: i32,
    pub item_count: i32,
}

fn parse_pagination_headers(headers: &reqwest::header::HeaderMap) -> Pagination {
    let int_or_zero = |key: &str| -> i32 {
        headers.get(key)
            .and_then(|v| v.to_str().ok())
            .and_then(|s| s.parse().ok())
            .unwrap_or(0)
    };
    
    Pagination {
        page: int_or_zero("X-Pagination-Page"),
        limit: int_or_zero("X-Pagination-Limit"),
        page_count: int_or_zero("X-Pagination-Page-Count"),
        item_count: int_or_zero("X-Pagination-Item-Count"),
    }
}

#[derive(Debug, Serialize, Deserialize)]
pub struct AuthDeviceCodeReq {
    pub client_id: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct AuthDeviceCodeResp {
    pub device_code: String,
    pub user_code: String,
    pub verification_url: String,
    pub expires_in: i32,
    pub interval: i32,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct AuthDeviceTokenReq {
    pub code: String,
    pub client_id: String,
    pub client_secret: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct AuthTokenReq {
    pub refresh_token: String,
    pub client_id: String,
    pub client_secret: String,
    pub redirect_uri: String,
    pub grant_type: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct AuthTokenResp {
    pub access_token: String,
    pub token_type: String,
    pub expires_in: i32,
    pub refresh_token: String,
    pub scope: String,
    pub created_at: i32,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct IDs {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub trakt: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub slug: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tvdb: Option<serde_json::Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub imdb: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tmdb: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tvrage: Option<serde_json::Value>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct HistoryItem {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub id: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub watched_at: Option<chrono::DateTime<chrono::Utc>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub action: Option<String>,
    #[serde(rename = "type")]
    #[serde(skip_serializing_if = "Option::is_none")]
    pub item_type: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub rated_at: Option<chrono::DateTime<chrono::Utc>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub rating: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub movie: Option<MovieInfo>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub episode: Option<EpisodeInfo>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub show: Option<ShowInfo>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MovieInfo {
    pub title: String,
    pub year: i32,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub ids: Option<IDs>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct EpisodeInfo {
    pub season: i32,
    pub number: i32,
    pub title: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub ids: Option<IDs>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ShowInfo {
    pub title: String,
    pub year: i32,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub ids: Option<IDs>,
}

impl HistoryItem {
    pub fn ids(&self) -> Option<IDs> {
        match self.item_type.as_deref() {
            Some("episode") => self.episode.as_ref()?.ids.clone(),
            Some("movie") => self.movie.as_ref()?.ids.clone(),
            Some("show") => self.show.as_ref()?.ids.clone(),
            _ => None,
        }
    }
}

impl std::fmt::Display for HistoryItem {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self.item_type.as_deref() {
            Some("episode") => {
                if let (Some(show), Some(episode)) = (&self.show, &self.episode) {
                    write!(f, "{} {}x{:02} {:?}", show.title, episode.season, episode.number, episode.title)
                } else {
                    write!(f, "Unknown episode")
                }
            }
            Some("movie") => {
                if let Some(movie) = &self.movie {
                    write!(f, "{} ({:04})", movie.title, movie.year)
                } else {
                    write!(f, "Unknown movie")
                }
            }
            _ => write!(f, "unknown media type {:?}", self.item_type),
        }
    }
}

pub type UserHistory = Vec<HistoryItem>;

pub trait UserHistoryExt {
    fn assign_ratings(&mut self, ratings: &UserHistory);
}

impl UserHistoryExt for UserHistory {
    fn assign_ratings(&mut self, ratings: &UserHistory) {
        for history_item in self.iter_mut() {
            if let Some(hi_ids) = history_item.ids() {
                if let Some(hi_trakt) = hi_ids.trakt {
                    for rating in ratings {
                        if history_item.item_type == rating.item_type {
                            if let Some(rating_ids) = rating.ids() {
                                if rating_ids.trakt == Some(hi_trakt) {
                                    history_item.rating = rating.rating;
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}

#[derive(Debug, Serialize, Deserialize)]
pub struct RatingResponse {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub added: Option<RatingAdded>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub not_found: Option<RatingNotFound>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct RatingAdded {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub episodes: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub movies: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub seasons: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub shows: Option<i32>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct RatingNotFound {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub episodes: Option<Vec<serde_json::Value>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub movies: Option<Vec<MovieNotFound>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub seasons: Option<Vec<serde_json::Value>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub shows: Option<Vec<serde_json::Value>>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct MovieNotFound {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub ids: Option<MovieNotFoundIds>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub rating: Option<i32>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct MovieNotFoundIds {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub imdb: Option<String>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct UserRatings {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub movies: Option<Vec<TraktMovie>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub episodes: Option<Vec<TraktEpisode>>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct UserSettings {
    pub user: UserInfo,
    pub account: AccountInfo,
    pub connections: ConnectionsInfo,
    pub sharing_text: SharingTextInfo,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct UserInfo {
    pub username: String,
    pub private: bool,
    pub name: String,
    pub vip: bool,
    pub vip_ep: bool,
    pub ids: UserIds,
    pub joined_at: chrono::DateTime<chrono::Utc>,
    pub location: String,
    pub about: String,
    pub gender: String,
    pub age: i32,
    pub images: UserImages,
    pub vip_og: bool,
    pub vip_years: i32,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct UserIds {
    pub slug: String,
    pub uuid: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct UserImages {
    pub avatar: AvatarInfo,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct AvatarInfo {
    pub full: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct AccountInfo {
    pub timezone: String,
    pub date_format: String,
    pub time_24hr: bool,
    pub cover_image: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct ConnectionsInfo {
    pub facebook: bool,
    pub twitter: bool,
    pub google: bool,
    pub tumblr: bool,
    pub medium: bool,
    pub slack: bool,
    pub apple: bool,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct SharingTextInfo {
    pub watching: String,
    pub watched: String,
    pub rated: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TraktMovie {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub rating: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub rated_at: Option<chrono::DateTime<chrono::Utc>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub title: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub year: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub ids: Option<IDs>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TraktShow {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub title: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub year: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub ids: Option<IDs>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TraktEpisode {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub season: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub number: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub title: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub ids: Option<IDs>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub overview: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub first_aired: Option<chrono::DateTime<chrono::Utc>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub updated_at: Option<chrono::DateTime<chrono::Utc>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub rated_at: Option<chrono::DateTime<chrono::Utc>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub rating: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub votes: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub comment_count: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub available_translations: Option<Vec<String>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub runtime: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub episode_type: Option<String>,
}

impl std::fmt::Display for TraktEpisode {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(
            f,
            "[{}]: {}x{:02} {:?}",
            self.ids.as_ref().and_then(|ids| ids.trakt).unwrap_or(0),
            self.season.unwrap_or(0),
            self.number.unwrap_or(0),
            self.title.as_deref().unwrap_or("Unknown")
        )
    }
}

pub type TraktResponse = Vec<TraktItem>;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TraktItem {
    #[serde(rename = "type")]
    pub item_type: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub score: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub progress: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub episode: Option<TraktEpisode>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub show: Option<TraktShow>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub movie: Option<TraktMovie>,
}

impl TraktItem {
    pub fn ids(&self) -> Option<IDs> {
        match self.item_type.as_str() {
            "episode" => self.episode.as_ref()?.ids.clone(),
            "movie" => self.movie.as_ref()?.ids.clone(),
            "show" => self.show.as_ref()?.ids.clone(),
            _ => None,
        }
    }
    
    pub fn matches(&self, guess: &Guess) -> bool {
        match guess.media_type.as_str() {
            "movie" => {
                guess.year == 0 || self.movie.as_ref().and_then(|m| m.year) == Some(guess.year)
            }
            "episode" | "show" => {
                guess.year == 0 || self.show.as_ref().and_then(|s| s.year) == Some(guess.year)
            }
            _ => false,
        }
    }
}

impl std::fmt::Display for TraktItem {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self.item_type.as_str() {
            "movie" => {
                if let Some(movie) = &self.movie {
                    write!(f, "{} ({:04})", movie.title.as_deref().unwrap_or("Unknown"), movie.year.unwrap_or(0))
                } else {
                    write!(f, "Unknown movie")
                }
            }
            "episode" => {
                if let (Some(show), Some(episode)) = (&self.show, &self.episode) {
                    write!(
                        f,
                        "{} {}x{:02} {:?} ({:04})",
                        show.title.as_deref().unwrap_or("Unknown"),
                        episode.season.unwrap_or(0),
                        episode.number.unwrap_or(0),
                        episode.title.as_deref().unwrap_or("Unknown"),
                        show.year.unwrap_or(0)
                    )
                } else {
                    write!(f, "Unknown episode")
                }
            }
            "show" => {
                if let Some(show) = &self.show {
                    write!(f, "{} ({:04})", show.title.as_deref().unwrap_or("Unknown"), show.year.unwrap_or(0))
                } else {
                    write!(f, "Unknown show")
                }
            }
            _ => write!(f, "Unknown media type: {:?}", self.item_type),
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ScrobbleItem {
    #[serde(flatten)]
    pub item: TraktItem,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub id: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub action: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub progress: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub sharing: Option<SharingInfo>,
    #[serde(rename = "type")]
    #[serde(skip_serializing)]
    pub item_type: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SharingInfo {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub twitter: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub mastodon: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tumblr: Option<bool>,
}

impl ScrobbleItem {
    pub fn media_kind(&self) -> String {
        if self.item.show.is_some() && self.item.episode.as_ref().and_then(|e| e.number).unwrap_or(0) <= 0 {
            return "show".to_string();
        }
        if self.item.movie.is_some() {
            return "movie".to_string();
        } else if self.item.episode.as_ref().and_then(|e| e.number).unwrap_or(0) > 0 {
            return "episode".to_string();
        }
        self.item_type.clone()
    }
}

// Guess struct referenced in the Go code
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Guess {
    pub title: String,
    #[serde(rename = "type")]
    pub media_type: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub episode_title: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub container: Option<String>,
    #[serde(default)]
    pub part: i32,
    #[serde(default)]
    pub episode: i32,
    #[serde(default)]
    pub season: i32,
    #[serde(default)]
    pub year: i32,
}

// Utility functions
pub fn json_dump<T: Serialize>(v: &T) -> Result<String> {
    Ok(serde_json::to_string_pretty(v)?)
}

pub fn json_dump_file<T: Serialize>(v: &T, filename: &str) -> Result<()> {
    let content = serde_json::to_string_pretty(v)?;
    fs::write(filename, content)?;
    Ok(())
}
