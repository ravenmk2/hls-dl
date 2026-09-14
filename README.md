# hls-dl

[![Test](https://github.com/ravenmk2/hls-dl/actions/workflows/test.yml/badge.svg)](https://github.com/ravenmk2/hls-dl/actions/workflows/test.yml)
[![Release](https://github.com/ravenmk2/hls-dl/actions/workflows/release.yml/badge.svg)](https://github.com/ravenmk2/hls-dl/actions/workflows/release.yml)
[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go)](https://go.dev/)

A fast HLS (m3u8) stream downloader that fetches all segments locally, then converts them to MP4 with FFmpeg.

## Features

- **Automatic retry** — exponential backoff on every segment; retries the entire pipeline until you press Ctrl+C
- **Concurrent downloads** — configurable worker pool for fast segment fetching
- **AES-128-CBC decryption** — handles encrypted HLS streams
- **Master playlist support** — auto-selects highest bitrate variant (or pick manually)
- **Custom HTTP headers** — for Referer, Cookie, Authorization, etc.
- **Progress bar** — real-time segment download progress

## Requirements

- [Go](https://go.dev/dl/) 1.21+
- [FFmpeg](https://ffmpeg.org/) (available in `$PATH`)

## Install

```bash
git clone https://github.com/ravenmk2/hls-dl.git
cd hls-dl
go build -o hls-dl .
```

## Usage

```bash
# Basic
hls-dl https://example.com/stream.m3u8

# With output path and concurrency
hls-dl -o video.mp4 -c 10 https://example.com/stream.m3u8

# With custom headers (e.g. Referer + Cookie)
hls-dl -H "Referer: https://example.com" -H "Cookie: session=abc" https://example.com/stream.m3u8

# Custom temp directory (auto-creates subdirectory)
hls-dl -t ./downloads https://example.com/stream.m3u8

# Keep temp files, custom User-Agent
hls-dl -k -u "Mozilla/5.0 ..." https://example.com/stream.m3u8
```

## Flags

| Flag | Alias | Default | Description |
|------|-------|---------|-------------|
| `--output` | `-o` | (inferred) | Output MP4 file path |
| `--temp-dir` | `-t` | system temp | Root directory for temporary segment files |
| `--concurrency` | `-c` | `5` | Number of concurrent download workers |
| `--header` | `-H` | — | Custom HTTP header (`Key:Value`), repeatable |
| `--variant-idx` | — | `-1` (best) | Variant index from master playlist |
| `--keep-temp` | `-k` | `false` | Keep temporary .ts files after conversion |
| `--user-agent` | `-u` | Chrome UA | User-Agent header |

## How it works

```txt
1. Fetch & parse m3u8 playlist
   ├── Master playlist? → pick best variant
   └── Media playlist? → extract segments & keys
2. Download all segments concurrently (with progress bar)
3. Decrypt segments if AES-128 encrypted
4. Generate FFmpeg concat file list
5. Run ffmpeg to merge into MP4
6. Clean up temp files (unless -k)
```

Any failure — dead segment, timeout, ffmpeg crash — triggers a full retry from step 1.
