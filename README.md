# trakt-cli

```

████████╗██████╗  █████╗ ██╗  ██╗████████╗     ██████╗██╗     ██╗
╚══██╔══╝██╔══██╗██╔══██╗██║ ██╔╝╚══██╔══╝    ██╔════╝██║     ██║
   ██║   ██████╔╝███████║█████╔╝    ██║       ██║     ██║     ██║
   ██║   ██╔══██╗██╔══██║██╔═██╗    ██║       ██║     ██║     ██║
   ██║   ██║  ██║██║  ██║██║  ██╗   ██║       ╚██████╗███████╗██║
   ╚═╝   ╚═╝  ╚═╝╚═╝  ╚═╝╚═╝  ╚═╝   ╚═╝        ╚═════╝╚══════╝╚═╝
```
# !!WARNING!! WORK IN PROGRESS

things might not work
~ scrobbling seems to work fine (further testing needed)
  the entire scrobbling code is a sketch and has to be refactored.

! Discord Rich Presense code is a mess but it works for me.
! `trakt-cli mv` does NOT work (it's an experiment and I might remove it)

This is a CLI for [trakt.tv](https://trakt.tv) using the [trakt.tv API](https://trakt.docs.apiary.io/).

![](https://user-images.githubusercontent.com/11699655/154494260-d3ff23ec-72b2-45e4-9f39-41f52119621b.png)

## Installation

Grab a binary build from the [releases](https://github.com/shizeeg/trakt-cli/releases).

## Development

```
git clone https://github.com/shizeeg/trakt-cli
cd trakt-cli
go build
```

## Usage

```
➜  trakt
Source code: https://github.com/shizeeg/trakt-cli

Usage:
  trakt-cli [command]

Available Commands:
  auth        Authenticate with trakt.tv
  completion  Generate the autocompletion script for the specified shell
  help        Help about any command
  history     Show your watched history

Flags:
  -h, --help     help for trakt-cli

Use "trakt-cli [command] --help" for more information about a command.
```

## Authentication

You need to create a _Trakt API app_ to use the API.

Go to https://trakt.tv/oauth/applications/new and create a new app.

This will give you a _Client ID_ and _Client secret_ for your app.

You can now log in with the CLI:

```
➜  trakt auth --client-id xxx --client-secret yyy
Please go to https://trakt.tv/activate and enter the following code: XXXXXXXX
Successfully authenticated, creds written to ~/.trakt.yaml
```
```
open https://discord.com/developers/applications?new_application=true
in your web browser, create a new App (the name will be shown as presense),
copy "ApplicationID" number add a line in your config:
`discord-appid: <your_number>`
save the file.
```
## TODO:
~ Discord Rich Presense: fix time. try to use `conn.Get("time-remaining")`
+ trakt-cli auth doesn't update the expired access-key.
