# CartmanCLI

CartmanCLI is a terminal-based episode launcher written in Go.

Browse episodes from a TUI, search by title, resume your last watched episode, and launch playback with `mpv`.

## Features

- Interactive TUI built with Bubble Tea
- Direct episode playback from the CLI
- Search by English episode title
- List episodes by season
- Resume last watched episode
- Local cache for faster search and playback
- `mpv` watch-later support
- Reset local data when needed

## Requirements

- `mpv`
- Go, only if building from source

On Arch Linux:

```bash
sudo pacman -S mpv
```

## Installation

Download the latest release archive, then:

```bash
tar -xzf cartman-linux-amd64.tar.gz
chmod +x cartman
sudo mv cartman /usr/local/bin/cartman
```

Check that it works:

```bash
cartman version
```

## Usage

Launch the interactive TUI:

```bash
cartman
```

Play an episode:

```bash
cartman play 10 8
```

Resume the last watched episode:

```bash
cartman resume
```

List a season:

```bash
cartman list 7
```

Search by title:

```bash
cartman search "warcraft"
cartman search "black friday"
cartman search "tegridy"
```

Build or refresh the local cache:

```bash
cartman update
```

Reset local data:

```bash
cartman reset
```

## Recommended setup

Run this once after installation:

```bash
cartman update
```

This builds a local cache of episode pages and video embeds. After that, search and playback are much faster.

## Local data

CartmanCLI stores data in standard user directories:

```txt
~/.config/cartmancli/
~/.cache/cartmancli/
```

To delete all local data:

```bash
cartman reset
```

Or skip confirmation:

```bash
cartman reset --yes
```

## Build from source

```bash
git clone git@github.com:Ybucaille/CartmanCLI.git
cd CartmanCLI
go build -o cartman ./cmd/cartman
```

Run it:

```bash
./cartman
```

## Notes

CartmanCLI depends on external episode pages and video providers. Availability may vary depending on the source pages and embeds.