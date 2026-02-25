# Rust API Reimplementation

This is a Rust reimplementation of the `api.go` module from the trakt-cli project.

## Overview

The module provides a complete async HTTP client for the Trakt.tv API v2 with the following features:

- **Authentication**: OAuth device code flow and token refresh
- **History Management**: Get user watch history with ratings
- **Rating Operations**: Add and retrieve user ratings
- **Search**: Query the Trakt database for movies, shows, and episodes
- **Scrobbling**: Start, stop, and pause scrobbling for currently watching media
- **User Settings**: Retrieve user profile and settings

## Key Differences from Go Version

### 1. **Async/Await**
The Rust implementation uses `async/await` with the `tokio` runtime, whereas the Go version uses synchronous HTTP calls:

```rust
// Rust
pub async fn get_user_history(&mut self, user: &str, params: PaginationParams) -> Result<(UserHistory, Pagination)>

// Go (for comparison)
func (c *APIClient) GetUserHistory(user string, params PaginationsParams) (resp UserHistory, pagination Pagination, err error)
```

### 2. **Error Handling**
Uses the `anyhow` crate for flexible error handling instead of Go's multiple return values:

```rust
// Rust
Result<T>  // Single return type with error handling

// Go
(result T, err error)  // Multiple returns
```

### 3. **Type Safety**
All fields are properly typed with `Option<T>` for optional fields:

```rust
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct IDs {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub trakt: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub slug: String,
    // ...
}
```

### 4. **Config Management**
Uses standard Rust configuration directories via the `dirs` crate and TOML deserialization.

### 5. **Token Refresh**
Automatic token refresh on 401 responses is preserved, with recursive retry after obtaining new tokens.

## Dependencies

```toml
reqwest = { version = "0.11", features = ["json"] }  # HTTP client
tokio = { version = "1", features = ["full"] }       # Async runtime
serde = { version = "1.0", features = ["derive"] }   # Serialization
serde_json = "1.0"                                   # JSON support
toml = "0.8"                                         # Config parsing
anyhow = "1.0"                                       # Error handling
chrono = { version = "0.4", features = ["serde"] }   # Date/time
dirs = "5.0"                                         # Config directories
```

## Usage Example

```rust
use trakt_api::{APIClient, PaginationParams};

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    // Create client (loads credentials from config)
    let mut client = APIClient::new()?;
    
    // Get user history with pagination
    let params = PaginationParams { page: 1, limit: 10 };
    let (history, pagination) = client.get_user_history("", params).await?;
    
    println!("Found {} items", history.len());
    for item in history {
        println!("{}", item);
    }
    
    Ok(())
}
```

## API Methods

### Authentication
- `new()` - Create client from config file
- `with_credentials(credentials)` - Create client with custom credentials
- `auth_device_code(req)` - Start device authentication flow
- `auth_device_token(req)` - Poll for device authentication completion
- `refresh_token(req)` - Manually refresh access token

### History & Ratings
- `get_user_history(user, params)` - Get watch history
- `get_history_with_ratings(params)` - Get history with ratings merged
- `get_ratings(params)` - Get user ratings
- `add_ratings(ratings)` - Add new ratings

### Search
- `trakt_query(query, media_type)` - Raw search query
- `trakt_search(guess)` - Smart search with fallback
- `episode_summary(show_id, season, episode)` - Get episode details

### Scrobbling
- `trakt_scrobble_start(item)` - Start watching
- `trakt_scrobble_pause(item)` - Pause watching
- `trakt_scrobble_stop(item)` - Stop watching

### User
- `get_user_settings()` - Get user profile settings

## Types

All major types from the Go version are preserved:

- `Credentials` - API credentials
- `HistoryItem` - Watch history entry
- `TraktItem` - Search result
- `TraktMovie`, `TraktShow`, `TraktEpisode` - Media types
- `ScrobbleItem` - Scrobble response
- `UserSettings` - User profile
- `Pagination`, `PaginationParams` - Pagination support
- `Guess` - Media metadata guess

## Testing

To build and check the module:

```bash
cargo build
cargo check
```

## Notes

1. The config file location follows XDG standards on Linux/Unix and standard locations on other platforms
2. Token refresh is automatic when receiving 401 responses
3. All methods are async and require an async runtime (tokio)
4. The module uses structured error types with `anyhow::Result`
5. Display traits are implemented for pretty printing media items
