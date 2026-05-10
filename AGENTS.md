# AGENTS.md

## Build & Verify

```sh
go build -o hls-dl .
go vet ./...
```

## Project Layout

```txt
main.go                     # CLI entry (urfave/cli)
internal/hls/               # M3U8 parsing, HTTP fetching
internal/downloader/        # Concurrent segment download + AES-128-CBC decrypt
internal/converter/         # FFmpeg concat -> MP4
```

No tests yet.

## Conventions

- Minimal dependencies: `urfave/cli/v2`, `grafov/m3u8`, `schollz/progressbar/v3`
- Exponential backoff retry (1s -> 60s max, resets on SIGINT)
- All stderr for status, stdout for user data (currently unused)
