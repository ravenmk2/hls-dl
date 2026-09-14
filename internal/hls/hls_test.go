package hls

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

const testMediaPlaylist = `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:10
#EXT-X-MEDIA-SEQUENCE:7
#EXTINF:9.9,
seg7.ts
#EXTINF:9.9,
seg8.ts
#EXTINF:9.9,
https://cdn.example.com/abs/seg9.ts
#EXT-X-ENDLIST
`

const testKeyPlaylist = `#EXTM3U
#EXT-X-TARGETDURATION:10
#EXT-X-MEDIA-SEQUENCE:3
#EXT-X-KEY:METHOD=AES-128,URI="keys/key.bin",IV=0x0000000000000000000000000000002a
#EXTINF:10,
a.ts
#EXT-X-KEY:METHOD=AES-128,URI="keys/key.bin"
#EXTINF:10,
b.ts
#EXT-X-ENDLIST
`

const testMasterPlaylist = `#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=800000,RESOLUTION=640x360,CODECS="avc1.42e01e"
low/index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=5000000,RESOLUTION=1920x1080,CODECS="avc1.640028"
high/index.m3u8
`

func testFetcher() *Fetcher {
	return NewFetcher(nil, "hls-dl-test", 30*time.Second)
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse URL %q: %v", raw, err)
	}
	return u
}

func TestResolveURL(t *testing.T) {
	base := mustURL(t, "https://example.com/live/index.m3u8")

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"relative file", "seg1.ts", "https://example.com/live/seg1.ts"},
		{"relative subdir", "chunks/seg1.ts", "https://example.com/live/chunks/seg1.ts"},
		{"root relative", "/media/seg1.ts", "https://example.com/media/seg1.ts"},
		{"absolute", "https://cdn.example.com/seg1.ts", "https://cdn.example.com/seg1.ts"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveURL(tt.raw, base)
			if err != nil {
				t.Fatalf("resolveURL: %v", err)
			}
			if got.String() != tt.want {
				t.Errorf("got %q, want %q", got.String(), tt.want)
			}
		})
	}
}

func TestComputeIV(t *testing.T) {
	iv := computeIV(0)
	if len(iv) != 16 {
		t.Fatalf("IV length = %d, want 16", len(iv))
	}
	for i, b := range iv {
		if b != 0 {
			t.Errorf("computeIV(0)[%d] = %d, want 0", i, b)
		}
	}

	iv = computeIV(258)
	want := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2}
	for i := range want {
		if iv[i] != want[i] {
			t.Fatalf("computeIV(258) = %v, want %v", iv, want)
		}
	}
}

func TestParseMediaPlaylist(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, testMediaPlaylist)
	}))
	defer srv.Close()

	info, err := ParseAndFetchVariant(context.Background(), testFetcher(), mustURL(t, srv.URL+"/live/index.m3u8"), -1)
	if err != nil {
		t.Fatalf("ParseAndFetchVariant: %v", err)
	}

	if !info.EndList {
		t.Error("EndList = false, want true")
	}
	if info.TargetDuration != 10 {
		t.Errorf("TargetDuration = %v, want 10", info.TargetDuration)
	}
	if len(info.Segments) != 3 {
		t.Fatalf("len(Segments) = %d, want 3", len(info.Segments))
	}

	for i, seg := range info.Segments {
		wantSeq := uint64(7 + i)
		if seg.SeqNum != wantSeq {
			t.Errorf("segment %d SeqNum = %d, want %d", i, seg.SeqNum, wantSeq)
		}
		wantName := fmt.Sprintf("seg_%06d.ts", wantSeq)
		if seg.FileName != wantName {
			t.Errorf("segment %d FileName = %q, want %q", i, seg.FileName, wantName)
		}
		if seg.Key != nil {
			t.Errorf("segment %d Key = %+v, want nil", i, seg.Key)
		}
	}

	if got := info.Segments[0].URL.String(); got != srv.URL+"/live/seg7.ts" {
		t.Errorf("segment 0 URL = %q, want %q", got, srv.URL+"/live/seg7.ts")
	}
	if got := info.Segments[2].URL.String(); got != "https://cdn.example.com/abs/seg9.ts" {
		t.Errorf("segment 2 URL = %q, want absolute URL preserved", got)
	}
}

func TestParseMediaPlaylistWithKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, testKeyPlaylist)
	}))
	defer srv.Close()

	info, err := ParseAndFetchVariant(context.Background(), testFetcher(), mustURL(t, srv.URL+"/index.m3u8"), -1)
	if err != nil {
		t.Fatalf("ParseAndFetchVariant: %v", err)
	}
	if len(info.Segments) != 2 {
		t.Fatalf("len(Segments) = %d, want 2", len(info.Segments))
	}

	segA := info.Segments[0]
	if segA.Key == nil {
		t.Fatal("segment 0 Key = nil, want AES-128 key")
	}
	if segA.Key.Method != "AES-128" {
		t.Errorf("Method = %q, want AES-128", segA.Key.Method)
	}
	if want := srv.URL + "/keys/key.bin"; segA.Key.URI != want {
		t.Errorf("Key URI = %q, want %q", segA.Key.URI, want)
	}
	if len(segA.Key.IV) != 16 || segA.Key.IV[15] != 0x2a {
		t.Errorf("IV = %x, want 16 bytes ending in 2a", segA.Key.IV)
	}

	segB := info.Segments[1]
	if segB.Key == nil {
		t.Fatal("segment 1 Key = nil, want inherited key")
	}
	wantIV := computeIV(4)
	if len(segB.Key.IV) != 16 {
		t.Fatalf("segment 1 IV length = %d, want 16", len(segB.Key.IV))
	}
	for i := range wantIV {
		if segB.Key.IV[i] != wantIV[i] {
			t.Fatalf("segment 1 IV = %x, want %x (from seqNum)", segB.Key.IV, wantIV)
		}
	}
}

func masterTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/master.m3u8", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, testMasterPlaylist)
	})
	mux.HandleFunc("/low/index.m3u8", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "#EXTM3U\n#EXT-X-TARGETDURATION:10\n#EXTINF:10,\nlow.ts\n#EXT-X-ENDLIST\n")
	})
	mux.HandleFunc("/high/index.m3u8", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "#EXTM3U\n#EXT-X-TARGETDURATION:10\n#EXTINF:10,\nhigh.ts\n#EXT-X-ENDLIST\n")
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestParseMasterPlaylistSelectsHighestBandwidth(t *testing.T) {
	srv := masterTestServer(t)

	info, err := ParseAndFetchVariant(context.Background(), testFetcher(), mustURL(t, srv.URL+"/master.m3u8"), -1)
	if err != nil {
		t.Fatalf("ParseAndFetchVariant: %v", err)
	}
	if got := info.Segments[0].URL.Path; got != "/high/high.ts" {
		t.Errorf("selected variant segment path = %q, want /high/high.ts", got)
	}
}

func TestParseMasterPlaylistUserIndex(t *testing.T) {
	srv := masterTestServer(t)

	info, err := ParseAndFetchVariant(context.Background(), testFetcher(), mustURL(t, srv.URL+"/master.m3u8"), 0)
	if err != nil {
		t.Fatalf("ParseAndFetchVariant: %v", err)
	}
	if got := info.Segments[0].URL.Path; got != "/low/low.ts" {
		t.Errorf("selected variant segment path = %q, want /low/low.ts", got)
	}
}

func TestListVariants(t *testing.T) {
	srv := masterTestServer(t)

	variants, err := ListVariants(context.Background(), testFetcher(), mustURL(t, srv.URL+"/master.m3u8"))
	if err != nil {
		t.Fatalf("ListVariants: %v", err)
	}
	if len(variants) != 2 {
		t.Fatalf("len(variants) = %d, want 2", len(variants))
	}
	if variants[0].Bandwidth != 800000 || variants[1].Bandwidth != 5000000 {
		t.Errorf("bandwidths = %d, %d; want 800000, 5000000", variants[0].Bandwidth, variants[1].Bandwidth)
	}
	if got := variants[1].URL.String(); got != srv.URL+"/high/index.m3u8" {
		t.Errorf("variant 1 URL = %q, want %q", got, srv.URL+"/high/index.m3u8")
	}
}

func TestListVariantsOnMediaPlaylist(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, testMediaPlaylist)
	}))
	defer srv.Close()

	variants, err := ListVariants(context.Background(), testFetcher(), mustURL(t, srv.URL+"/index.m3u8"))
	if err != nil {
		t.Fatalf("ListVariants: %v", err)
	}
	if variants != nil {
		t.Errorf("variants = %v, want nil for media playlist", variants)
	}
}

func TestFetcherSetsHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.UserAgent() != "test-agent" {
			http.Error(w, "bad UA", http.StatusBadRequest)
			return
		}
		if r.Header.Get("X-Auth") != "token123" {
			http.Error(w, "bad auth", http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	f := NewFetcher(map[string]string{"X-Auth": "token123"}, "test-agent", 30*time.Second)
	data, err := f.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if string(data) != "ok" {
		t.Errorf("body = %q, want %q", data, "ok")
	}
}

func TestFetcherRetriesAfterFailure(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, "recovered")
	}))
	defer srv.Close()

	data, err := testFetcher().Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if string(data) != "recovered" {
		t.Errorf("body = %q, want %q", data, "recovered")
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("server calls = %d, want 2", got)
	}
}

func TestFetcherRespectsContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if _, err := testFetcher().Fetch(ctx, srv.URL); err == nil {
		t.Fatal("Fetch returned nil error, want context deadline error")
	}
}
