package usecase

import (
	"accu/tracks"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
)

type Cfg struct {
	DownloadsRootDir string
}

type Usecase struct {
	tf  tracks.TracksFetcher
	cf  tracks.ChannelFetcher
	r   tracks.Repo
	c   *http.Client
	l   *log.Logger
	cfg Cfg
}

func New(cfg Cfg, rt http.RoundTripper, tf tracks.TracksFetcher, cf tracks.ChannelFetcher, r tracks.Repo, l *log.Logger) Usecase {
	return Usecase{
		tf,
		cf,
		r,
		&http.Client{
			Transport: rt,
		},
		l,
		cfg,
	}
}

func (u Usecase) Rip(ctx context.Context) error {
	handleErr := func(err error) error {
		return fmt.Errorf("usecase: do: %w", err)
	}
	channels, err := u.cf.FetchChannels()
	if err != nil {
		return handleErr(err)
	}
	wg := sync.WaitGroup{}
	wg.Add(len(channels))
	for _, ch := range channels {
		if err := u.r.SaveChannels(ctx, ch); err != nil {
			return handleErr(err)
		}
		go func(ch tracks.Channel) {
			var noNewTracks int
			defer wg.Done()
		loop:
			for {
				u.l.Printf("started fetching tracks for channel %s - %s", ch.DataId, ch.Name)
				select {
				case <-ctx.Done():
					break loop
				default:
				}
				trcks, err := u.tf.FetchTracks(tracks.FetchTracksParams{
					Channel: ch.DataId,
				})
				if err != nil {
					u.l.Print(err)
					continue
				}
				filtered, err := u.filterTracks(ctx, trcks)
				if err != nil {
					u.l.Print(err)
					continue
				}
				if len(filtered) == 0 {
					noNewTracks++
				} else {
					noNewTracks = 0
				}
				if noNewTracks == 100 {
					break loop
				}
				if err := u.r.SaveTracks(ctx, filtered...); err != nil {
					u.l.Print(err)
				}
				u.l.Printf("fetched %d tracks for channel %s - %s", len(filtered), ch.DataId, ch.Name)
			}
			u.l.Printf("exit fetching tracks for channel %s - %s", ch.DataId, ch.Name)
		}(ch)
	}
	wg.Wait()
	return nil
}

func (u Usecase) filterTracks(ctx context.Context, trcks []tracks.Track) ([]tracks.Track, error) {
	handleErr := func(err error) ([]tracks.Track, error) {
		return nil, fmt.Errorf("filter tracks: %w", err)
	}
	result := make([]tracks.Track, 0, len(trcks))
	for _, trck := range trcks {
		exists, err := u.trackExists(ctx, trck)
		if err != nil {
			return handleErr(err)
		}
		if exists {
			continue
		}
		result = append(result, trck)
	}
	return result, nil
}

func (u Usecase) trackExists(ctx context.Context, trck tracks.Track) (bool, error) {
	handleErr := func(err error) (bool, error) {
		return false, fmt.Errorf("track exists: %w", err)
	}
	if _, err := u.r.GetTrackByLink(ctx, trck.PrimaryLink); errors.Is(err, tracks.ErrNotFound) {
		return false, nil
	} else if err != nil {
		return handleErr(err)
	}
	if _, err := u.r.GetTrackByLink(ctx, trck.SecondaryLink); errors.Is(err, tracks.ErrNotFound) {
		return false, nil
	} else if err != nil {
		return handleErr(err)
	}
	return true, nil
}

func (u Usecase) Save(ctx context.Context) error {
	handleErr := func(err error) error {
		return fmt.Errorf("save tracks: %w", err)
	}
	sem := make(chan struct{}, 16)
	if err := u.r.GetAllTracks(ctx, func(ctx context.Context, t tracks.Track) error {
		sem <- struct{}{}
		go u.getTrack(ctx, t, sem)
		return nil
	}); err != nil {
		return handleErr(err)
	}
	return nil
}

func (u Usecase) getTrack(ctx context.Context, t tracks.Track, sem <-chan struct{}) {
	defer func() {
		<-sem
	}()
	filename := u.buildFileName(t)
	exists, err := isExist(filename)
	if err != nil {
		u.l.Print(err)
		return
	}
	if exists {
		u.l.Printf("track %q already exists", filename)
		return
	}
	if err := u.mkdir(t); err != nil {
		u.l.Print(err)
		return
	}
	outFile, err := os.Create(filename)
	if err != nil {
		u.l.Print(err)
		return
	}
	defer outFile.Close()
	from, err := u.downloadFile(t.PrimaryLink)
	if err != nil {
		from, err = u.downloadFile(t.SecondaryLink)
		if err != nil {
			u.l.Print(err)
			return
		}
	}
	defer from.Close()
	if _, err := io.Copy(outFile, from); err != nil {
		u.l.Print(err)
		return
	}
	u.l.Printf("saved %s", filename)
}

func (u Usecase) mkdir(t tracks.Track) error {
	name := cleanFilename(t.Channel)
	if err := os.MkdirAll(u.cfg.DownloadsRootDir+"/"+name, 0700); errors.Is(err, os.ErrExist) {
		return nil
	} else if err != nil {
		return err
	}
	return nil
}

func isExist(name string) (bool, error) {
	if _, err := os.Stat(name); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

func (u Usecase) buildFileName(t tracks.Track) string {
	channel := cleanFilename(t.Channel)
	artist := cleanFilename(t.Artist)
	album := cleanFilename(t.Album)
	year := strconv.Itoa(t.Year)
	title := cleanFilename(t.Title)
	// artist_-_album_-_year_-_title
	return fmt.Sprintf("%s/%s/%s_-_%s_-_%s_-_%s.m4a", u.cfg.DownloadsRootDir, channel, artist, album, year, title)
}

func cleanFilename(n string) string {
	return strings.ReplaceAll(n, "/", "_")
}

func (u Usecase) downloadFile(link string) (io.ReadCloser, error) {
	handleErr := func(err error) (io.ReadCloser, error) {
		return nil, fmt.Errorf("download file: %w", err)
	}
	resp, err := u.c.Get(link)
	if err != nil {
		return handleErr(err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return handleErr(fmt.Errorf("response status not ok: %q", resp.Status))
	}
	return resp.Body, nil
}
