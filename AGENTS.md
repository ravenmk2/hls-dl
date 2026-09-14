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
.github/workflows/          # CI: test on push/PR, release on v*.*.* tags
```

## CI

- `test.yml`: vet + build + test on push/PR to master, develop, main; matrix runs all 8 release targets natively (386 via GOARCH on amd64 runners, arm64 on ARM runners)
- `release.yml`: tag `v*.*.*` builds 8 targets (linux/windows/darwin x amd64/arm64, plus linux/windows 386) and publishes archives + checksums to GitHub Releases; version injected via `-X main.Version=<tag>`

## Conventions

- Minimal dependencies: `urfave/cli/v2`, `grafov/m3u8`, `schollz/progressbar/v3`
- Exponential backoff retry (1s -> 60s max, resets on SIGINT)
- All stderr for status, stdout for user data (currently unused)
