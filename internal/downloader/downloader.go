package downloader

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/ravenmk3/hls-dl/internal/hls"
	"github.com/schollz/progressbar/v3"
)

type Downloader struct {
	fetcher     *hls.Fetcher
	concurrency int
	tempDir     string
}

func New(fetcher *hls.Fetcher, concurrency int, tempDir string) *Downloader {
	return &Downloader{
		fetcher:     fetcher,
		concurrency: concurrency,
		tempDir:     tempDir,
	}
}

func (d *Downloader) Download(ctx context.Context, mediaPl *hls.MediaPlaylistInfo) ([]string, error) {
	total := len(mediaPl.Segments)
	if total == 0 {
		return nil, fmt.Errorf("no segments to download")
	}

	if err := d.fetchKeys(ctx, mediaPl); err != nil {
		return nil, fmt.Errorf("fetch keys: %w", err)
	}

	bar := progressbar.Default(int64(total))
	bar.Describe("Downloading")

	type job struct {
		idx     int
		segment *hls.SegmentInfo
	}

	jobs := make(chan job, total)
	errCh := make(chan error, 1)
	var wg sync.WaitGroup

	for w := 0; w < d.concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				if ctx.Err() != nil {
					return
				}

				destPath := fmt.Sprintf("%s%c%s", d.tempDir, os.PathSeparator, j.segment.FileName)
				if err := d.downloadSegment(ctx, j.segment, destPath); err != nil {
					select {
					case errCh <- fmt.Errorf("segment %d: %w", j.idx, err):
					default:
					}
					return
				}
				bar.Add(1)
			}
		}()
	}

	go func() {
	loop:
		for i, seg := range mediaPl.Segments {
			select {
			case jobs <- job{idx: i, segment: seg}:
			case <-ctx.Done():
				break loop
			}
		}
		close(jobs)
	}()

	wg.Wait()

	select {
	case err := <-errCh:
		return nil, err
	default:
	}

	var files []string
	for _, seg := range mediaPl.Segments {
		dst := fmt.Sprintf("%s%c%s", d.tempDir, os.PathSeparator, seg.FileName)
		files = append(files, dst)
	}
	return files, nil
}

func (d *Downloader) fetchKeys(ctx context.Context, mediaPl *hls.MediaPlaylistInfo) error {
	keyURIs := make(map[string]bool)
	for _, seg := range mediaPl.Segments {
		if seg.Key != nil && seg.Key.URI != "" {
			keyURIs[seg.Key.URI] = true
		}
	}

	fetchedKeys := make(map[string][]byte, len(keyURIs))
	for uri := range keyURIs {
		data, err := d.fetcher.Fetch(ctx, uri)
		if err != nil {
			return fmt.Errorf("fetch key %s: %w", uri, err)
		}
		fetchedKeys[uri] = data
	}

	for _, seg := range mediaPl.Segments {
		if seg.Key != nil && seg.Key.URI != "" {
			seg.Key.RawKey = fetchedKeys[seg.Key.URI]
		}
	}

	return nil
}

func (d *Downloader) downloadSegment(ctx context.Context, seg *hls.SegmentInfo, destPath string) error {
	delay := 1 * time.Second
	maxDelay := 60 * time.Second

	for {
		err := d.doSegmentDownload(ctx, seg, destPath)
		if err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			delay *= 2
			if delay > maxDelay {
				delay = maxDelay
			}
		}
	}
}

func (d *Downloader) doSegmentDownload(ctx context.Context, seg *hls.SegmentInfo, destPath string) error {
	raw, err := d.fetcher.Fetch(ctx, seg.URL.String())
	if err != nil {
		return err
	}

	if seg.Key != nil && len(seg.Key.RawKey) >= 16 {
		var err error
		raw, err = decryptAES128CBC(raw, seg.Key.RawKey, seg.Key.IV)
		if err != nil {
			return fmt.Errorf("decrypt: %w", err)
		}
	}

	return os.WriteFile(destPath, raw, 0644)
}

func decryptAES128CBC(data, key, iv []byte) ([]byte, error) {
	block, err := aes.NewCipher(key[:16])
	if err != nil {
		return nil, err
	}

	if len(data) < aes.BlockSize {
		return nil, fmt.Errorf("data too short: %d bytes", len(data))
	}

	if len(data)%aes.BlockSize != 0 {
		data = data[:len(data)-len(data)%aes.BlockSize]
	}

	mode := cipher.NewCBCDecrypter(block, iv[:16])
	out := make([]byte, len(data))
	mode.CryptBlocks(out, data)

	return out, nil
}
