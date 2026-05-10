package hls

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/grafov/m3u8"
)

func resolveURL(rawURL string, baseURL *url.URL) (*url.URL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse URL %q: %w", rawURL, err)
	}
	return baseURL.ResolveReference(u), nil
}

type FetchResponse struct {
	MediaPlaylist *MediaPlaylistInfo
	MasterPlayist *MasterPlaylist
}

type MasterPlaylist struct {
	Variants []*VariantInfo
}

func ParseAndFetch(ctx context.Context, fetcher *Fetcher, playlistURL *url.URL) (*MediaPlaylistInfo, error) {
	content, err := fetcher.FetchString(ctx, playlistURL.String())
	if err != nil {
		return nil, fmt.Errorf("fetch playlist: %w", err)
	}

	pl, listType, err := m3u8.DecodeFrom(strings.NewReader(content), true)
	if err != nil {
		return nil, fmt.Errorf("decode m3u8: %w", err)
	}

	switch listType {
	case m3u8.MEDIA:
		return parseMediaPlaylist(pl.(*m3u8.MediaPlaylist), playlistURL)
	case m3u8.MASTER:
		return parseMasterPlaylist(ctx, fetcher, pl.(*m3u8.MasterPlaylist), playlistURL, -1)
	default:
		return nil, fmt.Errorf("unknown playlist type: %d", listType)
	}
}

func ParseAndFetchVariant(ctx context.Context, fetcher *Fetcher, playlistURL *url.URL, variantIdx int) (*MediaPlaylistInfo, error) {
	content, err := fetcher.FetchString(ctx, playlistURL.String())
	if err != nil {
		return nil, fmt.Errorf("fetch playlist: %w", err)
	}

	pl, listType, err := m3u8.DecodeFrom(strings.NewReader(content), true)
	if err != nil {
		return nil, fmt.Errorf("decode m3u8: %w", err)
	}

	switch listType {
	case m3u8.MEDIA:
		return parseMediaPlaylist(pl.(*m3u8.MediaPlaylist), playlistURL)
	case m3u8.MASTER:
		return parseMasterPlaylist(ctx, fetcher, pl.(*m3u8.MasterPlaylist), playlistURL, variantIdx)
	default:
		return nil, fmt.Errorf("unknown playlist type: %d", listType)
	}
}

func ListVariants(ctx context.Context, fetcher *Fetcher, playlistURL *url.URL) ([]*VariantInfo, error) {
	content, err := fetcher.FetchString(ctx, playlistURL.String())
	if err != nil {
		return nil, err
	}

	pl, listType, err := m3u8.DecodeFrom(strings.NewReader(content), true)
	if err != nil {
		return nil, err
	}

	if listType == m3u8.MEDIA {
		return nil, nil
	}

	masterPl := pl.(*m3u8.MasterPlaylist)
	var variants []*VariantInfo
	for i, v := range masterPl.Variants {
		if v == nil {
			continue
		}
		variantURL, err := resolveURL(v.URI, playlistURL)
		if err != nil {
			return nil, fmt.Errorf("variant %d URL: %w", i, err)
		}
		variants = append(variants, &VariantInfo{
			URL:        variantURL,
			Bandwidth:  v.Bandwidth,
			Resolution: v.Resolution,
			Codecs:     v.Codecs,
		})
	}
	return variants, nil
}

func parseMasterPlaylist(ctx context.Context, fetcher *Fetcher, mp *m3u8.MasterPlaylist, baseURL *url.URL, userIdx int) (*MediaPlaylistInfo, error) {
	type variantWithIdx struct {
		variant *m3u8.Variant
		idx     int
	}

	var sorted []variantWithIdx
	for i, v := range mp.Variants {
		if v == nil || v.URI == "" {
			continue
		}
		sorted = append(sorted, variantWithIdx{v, i})
	}

	if len(sorted) == 0 {
		return nil, fmt.Errorf("no variants in master playlist")
	}

	sort.Slice(sorted, func(a, b int) bool {
		return sorted[a].variant.Bandwidth > sorted[b].variant.Bandwidth
	})

	selected := sorted[0]
	if userIdx >= 0 {
		for _, sv := range sorted {
			if sv.idx == userIdx {
				selected = sv
				break
			}
		}
	}

	variantURL, err := resolveURL(selected.variant.URI, baseURL)
	if err != nil {
		return nil, fmt.Errorf("resolve variant URL: %w", err)
	}

	content, err := fetcher.FetchString(ctx, variantURL.String())
	if err != nil {
		return nil, fmt.Errorf("fetch variant playlist: %w", err)
	}

	pl, listType, err := m3u8.DecodeFrom(strings.NewReader(content), true)
	if err != nil {
		return nil, fmt.Errorf("decode variant playlist: %w", err)
	}

	if listType != m3u8.MEDIA {
		return nil, fmt.Errorf("expected media playlist for variant, got type %d", listType)
	}

	return parseMediaPlaylist(pl.(*m3u8.MediaPlaylist), variantURL)
}

func parseMediaPlaylist(mp *m3u8.MediaPlaylist, baseURL *url.URL) (*MediaPlaylistInfo, error) {

	var keyStore *KeyInfo

	info := &MediaPlaylistInfo{
		TargetDuration: mp.TargetDuration,
		EndList:        mp.Closed,
	}

	var segIdx uint64
	for _, seg := range mp.Segments {
		if seg == nil {
			continue
		}

		seqNum := mp.SeqNo + segIdx
		segIdx++

		segURL, err := resolveURL(seg.URI, baseURL)
		if err != nil {
			return nil, fmt.Errorf("segment %d URL: %w", seqNum, err)
		}

		si := &SegmentInfo{
			URL:      segURL,
			SeqNum:   seqNum,
			Duration: seg.Duration,
			FileName: fmt.Sprintf("seg_%06d.ts", seqNum),
		}

		if seg.Key != nil && seg.Key.Method != "" && seg.Key.Method != "NONE" {
			if seg.Key.URI != "" {
				keyURL, err := resolveURL(seg.Key.URI, baseURL)
				if err != nil {
					return nil, fmt.Errorf("segment %d key URL: %w", seqNum, err)
				}
				keyStore = &KeyInfo{
					Method: seg.Key.Method,
					URI:    keyURL.String(),
				}
			} else {
				keyStore = &KeyInfo{
					Method: seg.Key.Method,
				}
			}

			if seg.Key.IV != "" {
				ivHex := strings.TrimPrefix(strings.TrimPrefix(seg.Key.IV, "0x"), "0X")
				ivBytes, err := hex.DecodeString(ivHex)
				if err == nil && len(ivBytes) == 16 {
					keyStore.IV = ivBytes
				}
			}
		}

		if keyStore != nil {
			keyCopy := *keyStore
			if keyCopy.IV == nil {
				keyCopy.IV = computeIV(seqNum)
			}
			si.Key = &keyCopy
		}

		info.Segments = append(info.Segments, si)
	}

	return info, nil
}

func computeIV(seqNum uint64) []byte {
	iv := make([]byte, 16)
	putBigEndianUint64(iv[8:], seqNum)
	return iv
}

func putBigEndianUint64(b []byte, v uint64) {
	binary.BigEndian.PutUint64(b, v)
}
