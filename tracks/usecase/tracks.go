package usecase

import (
	"accu/tracks"
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
)

type Usecase struct {
	tf tracks.TracksFetcher
	cf tracks.ChannelFetcher
	r  tracks.Repo
}

func New(tf tracks.TracksFetcher, cf tracks.ChannelFetcher, r tracks.Repo) Usecase {
	return Usecase{
		tf: tf,
		cf: cf,
		r:  r,
	}
}

func (u Usecase) Do(ctx context.Context) error {
	handleErr := func(err error) error {
		return fmt.Errorf("usecase: do: %w", err)
	}
	channels, err := u.cf.FetchChannels()
	if err != nil {
		return handleErr(err)
	}
	wg := sync.WaitGroup{}
	for _, ch := range channels {
		if err := u.r.SaveChannels(ctx, ch); err != nil {
			return handleErr(err)
		}
		wg.Add(1)
		go func(ch tracks.Channel) {
			var noNewTracks int
		loop:
			for {
				log.Printf("started fetching tracks for channel %s - %s", ch.DataId, ch.Name)
				select {
				case <-ctx.Done():
					break loop
				default:
				}
				trcks, err := u.tf.FetchTracks(tracks.FetchTracksParams{
					Channel: ch.DataId,
				})
				if err != nil {
					log.Print(err)
					continue
				}
				filtered, err := u.filterTracks(trcks)
				if err != nil {
					log.Print(err)
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
					log.Print(err)
				}
				log.Printf("fetched tracks for channel %s - %s", ch.DataId, ch.Name)
			}
			log.Printf("exit fetching tracks for channel %s - %s", ch.DataId, ch.Name)
			wg.Done()
		}(ch)
	}
	wg.Wait()
	return nil
}

func (u Usecase) filterTracks(trcks []tracks.Track) ([]tracks.Track, error) {
	handleErr := func(err error) ([]tracks.Track, error) {
		return nil, fmt.Errorf("filter tracks: %w", err)
	}
	result := make([]tracks.Track, 0, len(trcks))
	for _, trck := range trcks {
		exists, err := u.trackExists(trck)
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

func (u Usecase) trackExists(trck tracks.Track) (bool, error) {
	handleErr := func(err error) (bool, error) {
		return false, fmt.Errorf("track exists: %w", err)
	}
	if _, err := u.r.GetTrackByLink(trck.PrimaryLink); errors.Is(err, tracks.ErrNotFound) {
		return false, nil
	} else if err != nil {
		return handleErr(err)
	}
	if _, err := u.r.GetTrackByLink(trck.SecondaryLink); errors.Is(err, tracks.ErrNotFound) {
		return false, nil
	} else if err != nil {
		return handleErr(err)
	}
	return true, nil
}
