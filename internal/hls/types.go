package hls

import "net/url"

type SegmentInfo struct {
	URL      *url.URL
	SeqNum   uint64
	Duration float64
	FileName string
	Key      *KeyInfo
}

type KeyInfo struct {
	Method string
	URI    string
	IV     []byte
	RawKey []byte
}

type VariantInfo struct {
	URL        *url.URL
	Bandwidth  uint32
	Resolution string
	Codecs     string
}

type MediaPlaylistInfo struct {
	Segments       []*SegmentInfo
	TargetDuration float64
	EndList        bool
}
