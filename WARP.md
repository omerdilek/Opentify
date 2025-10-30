# WARP.md

This file provides guidance to WARP (warp.dev) when working with code in this repository.

Project overview
- Language: Go (module: opentify). Desktop GUI built with Fyne v2; desktop audio via faiface/beep. App scans music files under musicdb/ and plays them (desktop). Mobile (Android/iOS) player is a stub for now.
- High-level architecture:
  - UI (main.go): Fyne window with a List showing files from musicdb/, and controls: Play/Pause/Stop/Refresh. Selecting a file then calls player.Load(...) and player.Play().
  - Media scanning (main.go): Walks musicdb/ recursively, filters by .mp3/.wav/.flac/.ogg, keeps a sorted list.
  - Player abstraction (internal/player):
    - Desktop (player_desktop.go, build tag: !android && !ios): thread-safe Player with Load/Play/Pause/Stop/CurrentFile. Uses beep to decode formats (mp3/wav/flac/ogg) and speaker for playback; initializes speaker per file’s sample rate. Play starts the stream once and toggles paused state; Stop seeks to 0.
    - Mobile (player_mobile.go, build tag: android || ios): same API, no-op methods (skeleton for future implementation).

Common commands
- Run (desktop):
  ```bash
  go run .
  ```
- Build binary:
  ```bash
  go build -o opentify ./
  ./opentify
  ```
- Dependencies (tidy module file):
  ```bash
  go mod tidy
  ```
- Format and basic linting:
  ```bash
  go fmt ./...
  go vet ./...
  ```
- Tests (none yet present, but commands below apply as you add them):
  - All packages:
    ```bash
    go test ./...
    ```
  - Single package:
    ```bash
    go test ./internal/player
    ```
  - Single test by name (regex):
    ```bash
    go test -run '^TestName$' ./internal/player
    ```

Notes for agents
- Build tags govern platform behavior in internal/player: desktop builds use the beep-backed implementation; android/ios builds use the stub. Don’t assume mobile playback works yet.
- The musicdb/ directory is user media, not source. To speed indexing, consider excluding it via a .warpindexingignore entry:
  ```
  musicdb/
  ```
- No Makefile or CI present; prefer stock Go tooling (go run/build/test/fmt/vet).
