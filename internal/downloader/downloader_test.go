package downloader

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ravenmk3/hls-dl/internal/hls"
)

var (
	testKey = []byte("0123456789abcdef")
	testIV  = []byte("abcdef0123456789")
)

func encryptAES128CBC(t *testing.T, plaintext, key, iv []byte) []byte {
	t.Helper()
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	out := make([]byte, len(plaintext))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(out, plaintext)
	return out
}

func TestDecryptAES128CBC(t *testing.T) {
	plaintext := []byte("0123456789abcdefHLS-DL test data")
	ciphertext := encryptAES128CBC(t, plaintext, testKey, testIV)

	out, err := decryptAES128CBC(ciphertext, testKey, testIV)
	if err != nil {
		t.Fatalf("decryptAES128CBC: %v", err)
	}
	if !bytes.Equal(out, plaintext) {
		t.Errorf("roundtrip = %q, want %q", out, plaintext)
	}
}

func TestDecryptAES128CBCTruncatesNonBlockData(t *testing.T) {
	plaintext := []byte("0123456789abcdef")
	ciphertext := encryptAES128CBC(t, plaintext, testKey, testIV)
	withTail := append(ciphertext, 1, 2, 3, 4, 5)

	out, err := decryptAES128CBC(withTail, testKey, testIV)
	if err != nil {
		t.Fatalf("decryptAES128CBC: %v", err)
	}
	if !bytes.Equal(out, plaintext) {
		t.Errorf("got %q, want %q (tail truncated)", out, plaintext)
	}
}

func TestDecryptAES128CBCRejectsShortData(t *testing.T) {
	if _, err := decryptAES128CBC([]byte("short"), testKey, testIV); err == nil {
		t.Fatal("expected error for data shorter than one block")
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse URL %q: %v", raw, err)
	}
	return u
}

func verifyFiles(t *testing.T, files []string, want [][]byte) {
	t.Helper()
	if len(files) != len(want) {
		t.Fatalf("len(files) = %d, want %d", len(files), len(want))
	}
	for i, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if !bytes.Equal(data, want[i]) {
			t.Errorf("file %d %s = %q, want %q", i, filepath.Base(f), data, want[i])
		}
	}
}

func TestDownloadPlainSegments(t *testing.T) {
	contents := [][]byte{
		[]byte("segment-zero-content"),
		[]byte("segment-one-content!"),
		[]byte("segment-two-content?"),
	}

	mux := http.NewServeMux()
	for i, c := range contents {
		data := c
		mux.HandleFunc(fmt.Sprintf("/seg%d.ts", i), func(w http.ResponseWriter, r *http.Request) {
			w.Write(data)
		})
	}
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mediaPl := &hls.MediaPlaylistInfo{}
	for i := range contents {
		mediaPl.Segments = append(mediaPl.Segments, &hls.SegmentInfo{
			URL:      mustParseURL(t, fmt.Sprintf("%s/seg%d.ts", srv.URL, i)),
			SeqNum:   uint64(i),
			FileName: fmt.Sprintf("seg_%06d.ts", i),
		})
	}

	tempDir := t.TempDir()
	d := New(hls.NewFetcher(nil, "test", 30*time.Second), 2, tempDir)

	files, err := d.Download(context.Background(), mediaPl)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	verifyFiles(t, files, contents)

	for i, f := range files {
		want := filepath.Join(tempDir, fmt.Sprintf("seg_%06d.ts", i))
		if f != want {
			t.Errorf("files[%d] = %q, want %q", i, f, want)
		}
	}
}

func TestDownloadEncryptedSegments(t *testing.T) {
	plaintexts := [][]byte{
		[]byte("0123456789abcdef"),
		[]byte("fedcba9876543210"),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/key.bin", func(w http.ResponseWriter, r *http.Request) {
		w.Write(testKey)
	})
	for i, p := range plaintexts {
		data := encryptAES128CBC(t, p, testKey, testIV)
		mux.HandleFunc(fmt.Sprintf("/enc%d.ts", i), func(w http.ResponseWriter, r *http.Request) {
			w.Write(data)
		})
	}
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mediaPl := &hls.MediaPlaylistInfo{}
	for i := range plaintexts {
		iv := make([]byte, 16)
		copy(iv, testIV)
		mediaPl.Segments = append(mediaPl.Segments, &hls.SegmentInfo{
			URL:      mustParseURL(t, fmt.Sprintf("%s/enc%d.ts", srv.URL, i)),
			SeqNum:   uint64(i),
			FileName: fmt.Sprintf("seg_%06d.ts", i),
			Key: &hls.KeyInfo{
				Method: "AES-128",
				URI:    srv.URL + "/key.bin",
				IV:     iv,
			},
		})
	}

	d := New(hls.NewFetcher(nil, "test", 30*time.Second), 2, t.TempDir())

	files, err := d.Download(context.Background(), mediaPl)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	verifyFiles(t, files, plaintexts)
}

func TestDownloadNoSegments(t *testing.T) {
	d := New(hls.NewFetcher(nil, "test", 30*time.Second), 2, t.TempDir())
	if _, err := d.Download(context.Background(), &hls.MediaPlaylistInfo{}); err == nil {
		t.Fatal("expected error for empty segment list")
	}
}
