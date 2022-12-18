package fetcher

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"accu/tracks"
)

type Cfg struct {
	BaseURI string
}

type TrackListFetcher struct {
	cfg Cfg
	c   *http.Client
}

func NewTrackListFetcher(rt http.RoundTripper, cfg Cfg) TrackListFetcher {
	return TrackListFetcher{
		c: &http.Client{
			Transport: rt,
		},
		cfg: cfg,
	}
}

var _ tracks.TracksFetcher = TrackListFetcher{}

func (tlf TrackListFetcher) FetchTracks(p tracks.FetchTracksParams) ([]tracks.Track, error) {
	handleErr := func(err error) ([]tracks.Track, error) {
		return nil, fmt.Errorf("fetch tracks: %w", err)
	}
	uri := tlf.cfg.BaseURI + p.Channel + "/"
	resp, err := tlf.c.Get(uri)
	if err != nil {
		return handleErr(err)
	}
	defer resp.Body.Close()
	var rawTracks []rawTrack
	if err := json.NewDecoder(resp.Body).Decode(&rawTracks); err != nil {
		return handleErr(err)
	}
	tracks := make([]tracks.Track, 0, len(rawTracks))
	for _, rt := range rawTracks {
		tracks = append(tracks, rt.toTrack(p.Channel))
	}
	return tracks, nil
}

type rawAlbum struct {
	Title string
	Year  string
}

type rawTrack struct {
	Album       rawAlbum
	TrackArtist string `json:"track_artist"`
	Title,
	Primary,
	Secondary,
	Fn string
	Duration float64
}

func (r rawTrack) toTrack(channel string) tracks.Track {
	year, _ := strconv.Atoi(r.Album.Year)
	return tracks.Track{
		Channel:       channel,
		Artist:        r.TrackArtist,
		Album:         r.Album.Title,
		Title:         r.Title,
		Duration:      int(r.Duration),
		Year:          year,
		PrimaryLink:   r.Primary + r.Fn + ".m4a",
		SecondaryLink: r.Secondary + r.Fn + ".m4a",
	}
}
