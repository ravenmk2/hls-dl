package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"path"
	"strings"
	"time"

	"github.com/ravenmk3/hls-dl/internal/converter"
	"github.com/ravenmk3/hls-dl/internal/downloader"
	"github.com/ravenmk3/hls-dl/internal/hls"
	"github.com/urfave/cli/v2"
)

var Version = "dev"

func main() {
	app := &cli.App{
		Name:      "hls-dl",
		Usage:     "Download HLS (m3u8) streams and convert to MP4",
		Version:   Version,
		UsageText: "hls-dl [options] <m3u8-url> [output.mp4]",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "Output MP4 file path",
			},
			&cli.StringFlag{
				Name:    "temp-dir",
				Aliases: []string{"t"},
				Usage:   "Temporary directory for segment files",
			},
			&cli.IntFlag{
				Name:    "concurrency",
				Aliases: []string{"c"},
				Value:   5,
				Usage:   "Number of concurrent download workers",
			},
			&cli.StringSliceFlag{
				Name:    "header",
				Aliases: []string{"H"},
				Usage:   "Custom HTTP header (KEY:VALUE), can be specified multiple times",
			},
		&cli.IntFlag{
			Name:  "variant-idx",
			Value: -1,
			Usage: "Variant index for master playlists (-1 = auto-select best quality)",
		},
		&cli.BoolFlag{
			Name:    "keep-temp",
			Aliases: []string{"k"},
			Usage:   "Keep temporary files after conversion",
		},
		&cli.StringFlag{
			Name:    "user-agent",
			Aliases: []string{"u"},
			Value:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/132.0.0.0 Safari/537.36",
			Usage:   "User-Agent header",
		},
		},
		Action: run,
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(c *cli.Context) error {
	args := c.Args()
	if args.Len() < 1 {
		return fmt.Errorf("m3u8 URL is required")
	}

	m3u8URL, err := url.Parse(args.First())
	if err != nil {
		return fmt.Errorf("invalid URL %q: %w", args.First(), err)
	}

	outputPath := c.String("output")
	if outputPath == "" {
		if args.Len() >= 2 {
			outputPath = args.Get(1)
		} else {
			baseName := path.Base(m3u8URL.Path)
			if ext := path.Ext(baseName); ext != "" {
				baseName = baseName[:len(baseName)-len(ext)]
			}
			if baseName == "" || baseName == "." {
				baseName = "output"
			}
			outputPath = baseName + ".mp4"
		}
	}

	tempRoot := c.String("temp-dir")
	if tempRoot == "" {
		tempRoot = os.TempDir()
	}
	if err := os.MkdirAll(tempRoot, 0755); err != nil {
		return fmt.Errorf("create temp root: %w", err)
	}
	tempDir, err := os.MkdirTemp(tempRoot, "hls-dl-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}

	if !c.Bool("keep-temp") {
		defer os.RemoveAll(tempDir)
	} else {
		fmt.Fprintf(os.Stderr, "Temp dir: %s\n", tempDir)
	}

	headers := parseHeaders(c.StringSlice("header"))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	fetcher := hls.NewFetcher(headers, c.String("user-agent"))
	concurrency := c.Int("concurrency")

	attempt := 0
	for {
		attempt++

		select {
		case <-ctx.Done():
			return nil
		default:
		}

		if attempt > 1 {
			fmt.Fprintf(os.Stderr, "\n--- Retry attempt %d ---\n", attempt)
		}

		fmt.Fprintf(os.Stderr, "Fetching playlist: %s\n", m3u8URL.String())

		mediaPl, err := hls.ParseAndFetchVariant(ctx, fetcher, m3u8URL, c.Int("variant-idx"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "  error: %v, retrying...\n", err)
			time.Sleep(5 * time.Second)
			continue
		}

		fmt.Fprintf(os.Stderr, "Segments: %d, endlist: %v\n", len(mediaPl.Segments), mediaPl.EndList)

		dl := downloader.New(fetcher, concurrency, tempDir)

		fmt.Fprintf(os.Stderr, "Downloading segments...\n")
		segmentFiles, err := dl.Download(ctx, mediaPl)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			fmt.Fprintf(os.Stderr, "  error: %v, retrying...\n", err)
			time.Sleep(5 * time.Second)
			continue
		}

		fmt.Fprintf(os.Stderr, "\nConverting to MP4...\n")
		if err := converter.Convert(ctx, segmentFiles, outputPath); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			fmt.Fprintf(os.Stderr, "  error: %v, retrying...\n", err)
			time.Sleep(5 * time.Second)
			continue
		}

		fmt.Fprintf(os.Stderr, "\nSaved to: %s\n", outputPath)
		return nil
	}
}

func parseHeaders(raw []string) map[string]string {
	headers := make(map[string]string)
	for _, h := range raw {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) == 2 {
			headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return headers
}
