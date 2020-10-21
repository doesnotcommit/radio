package tracks

import (
	"context"
	"errors"
)

type Track struct {
	Channel,
	Artist,
	Album,
	Title,
	Year,
	PrimaryLink,
	SecondaryLink string
	Duration int
}

type Channel struct {
	Name,
	DataId string
}

type FetchTracksParams struct {
	Channel string
}

type TracksFetcher interface {
	FetchTracks(params FetchTracksParams) ([]Track, error)
}

type ChannelFetcher interface {
	FetchChannels() ([]Channel, error)
}

type Repo interface {
	SaveTracks(ctx context.Context, trks ...Track) error
	SaveChannels(ctx context.Context, chs ...Channel) error
	GetTrackByLink(link string) (Track, error)
	GetAllTracks(ctx context.Context, run func(t Track) error) error
}

var (
	ErrNotFound        = errors.New("not found")
	ErrDuplicateEntity = errors.New("duplicate entity")
)
