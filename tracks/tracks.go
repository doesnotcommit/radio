package tracks

import (
	"context"
	"errors"
)

type Track struct {
	Channel       string
	Artist        string
	Album         string
	Title         string
	Year          int
	PrimaryLink   string
	SecondaryLink string
	Duration      int
}

type Channel struct {
	Name   string
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
	GetTrackByLink(ctx context.Context, link string) (Track, error)
	GetAllTracks(ctx context.Context, run func(ctx context.Context, t Track) error) error
}

// TODO error interfaces
var (
	ErrNotFound        = errors.New("not found")
	ErrDuplicateEntity = errors.New("duplicate entity")
)
