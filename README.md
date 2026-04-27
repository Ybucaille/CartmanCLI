# CartmanCLI

CartmanCLI is a terminal-based South Park episode launcher written in Go.

## Features

- Browse seasons and episodes from a TUI
- Launch an episode directly from the CLI
- Resume the last watched episode
- Resolve real episode URLs from season pages
- Open videos with mpv
- Save watch progress locally

## Usage

```bash
go run ./cmd/cartman
go run ./cmd/cartman 1 8
go run ./cmd/cartman resume
Requirements
Go
mpv
Build
go build -o cartman ./cmd/cartman

Then run:

./cartman
./cartman 1 8
./cartman resume

Ensuite :

```bash
git add README.md
git commit -m "Update README"
git push
