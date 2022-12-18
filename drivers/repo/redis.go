package repo

import (
	"accu/drivers/repo/protos"
	"accu/tracks"
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	goredis "github.com/go-redis/redis/v9"
	"google.golang.org/protobuf/proto"
)

type Redis struct {
	client *goredis.Client
	l      *log.Logger
}

func NewRedisClient(ctx context.Context, host string, port int, l *log.Logger) (*goredis.Client, func(), error) {
	addr := fmt.Sprintf("%s:%d", host, port)
	client := goredis.NewClient(
		&goredis.Options{
			Network:         "tcp",
			Addr:            addr,
			DB:              0,
			MaxRetries:      3,
			MinRetryBackoff: 50 * time.Millisecond,
			MaxRetryBackoff: 2 * time.Second,
			DialTimeout:     10 * time.Second,
			ReadTimeout:     5 * time.Second,
			WriteTimeout:    5 * time.Second,
			MinIdleConns:    1,
			MaxIdleConns:    32,
			ConnMaxIdleTime: time.Minute,
			ConnMaxLifetime: time.Hour,
		},
	)
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, nil, fmt.Errorf("new redis client ping: %w", err)
	}
	return client, func() {
		if err := client.Close(); err != nil {
			l.Println(err)
		}
	}, nil
}

func NewRedis(client *goredis.Client, l *log.Logger) Redis {
	return Redis{
		client,
		l,
	}
}

func (r Redis) SaveTracks(ctx context.Context, trks ...tracks.Track) error {
	handleErr := func(err error) error {
		return fmt.Errorf("save tracks: %w", err)
	}
	cmds, err := r.client.Pipelined(ctx, func(pipe goredis.Pipeliner) error {
		for _, trk := range trks {
			trackMsg := protos.Track{
				Channel:       trk.Channel,
				Artist:        trk.Artist,
				Album:         trk.Album,
				Title:         trk.Title,
				Year:          int32(trk.Year),
				PrimaryLink:   trk.PrimaryLink,
				SecondaryLink: trk.SecondaryLink,
				Duration:      int32(trk.Duration),
			}
			rawTrackMsg, err := proto.Marshal(&trackMsg)
			if err != nil {
				return handleErr(err)
			}
			_ = pipe.HSet(ctx, "tracks", trk.PrimaryLink, rawTrackMsg)
			_ = pipe.HSet(ctx, "tracks", trk.SecondaryLink, rawTrackMsg)
			_ = pipe.SAdd(ctx, "trackprimarylinks", trk.PrimaryLink)
			_ = pipe.SAdd(ctx, "tracksecondsarylinks", trk.SecondaryLink)
			_ = pipe.LPush(ctx, fmt.Sprintf("channel:tracks:%s", trk.Channel), trk.PrimaryLink)
			_ = pipe.LPush(ctx, fmt.Sprintf("artist:tracks:%s", trk.Artist), trk.PrimaryLink)
			_ = pipe.LPush(ctx, fmt.Sprintf("artist:album:tracks:%s:%s", trk.Artist, trk.Album), trk.PrimaryLink)
			_ = pipe.LPush(ctx, fmt.Sprintf("year:tracks:%d", trk.Year), trk.PrimaryLink)
			_ = pipe.LPush(ctx, fmt.Sprintf("artist:year:tracks:%s:%d", trk.Artist, trk.Year), trk.PrimaryLink)
		}
		return nil
	})
	for _, cmd := range cmds {
		if cmd.Err(); err != nil {
			return handleErr(err)
		}
	}
	return nil
}

func (r Redis) SaveChannels(ctx context.Context, chs ...tracks.Channel) error {
	cmds, err := r.client.Pipelined(ctx, func(pipe goredis.Pipeliner) error {
		for _, ch := range chs {
			_ = pipe.HSet(ctx, "channels", map[string]any{
				"name":   ch.Name,
				"dataId": ch.DataId,
			})
		}
		return nil
	})
	for _, cmd := range cmds {
		if cmd.Err(); err != nil {
			return fmt.Errorf("save channels: %w", err)
		}
	}
	return nil
}

func (r Redis) GetTrackByLink(ctx context.Context, link string) (tracks.Track, error) {
	handleErr := func(err error) (tracks.Track, error) {
		return tracks.Track{}, fmt.Errorf("get track by link: %w", err)
	}
	rawTrack, err := r.client.HGet(ctx, "tracks", link).Bytes()
	if errors.Is(err, goredis.Nil) {
		return handleErr(tracks.ErrNotFound)
	} else if err != nil {
		return handleErr(err)
	}
	var trackMsg protos.Track
	if err := proto.Unmarshal(rawTrack, &trackMsg); err != nil {
		return handleErr(err)
	}
	trk := msgToTrack(&trackMsg)
	return trk, nil
}

func msgToTrack(msg *protos.Track) tracks.Track {
	return tracks.Track{
		Channel:       msg.Channel,
		Artist:        msg.Artist,
		Album:         msg.Album,
		Title:         msg.Title,
		Year:          int(msg.Year),
		PrimaryLink:   msg.PrimaryLink,
		SecondaryLink: msg.SecondaryLink,
		Duration:      int(msg.Duration),
	}

}

func (r Redis) GetAllTracks(ctx context.Context, run func(ctx context.Context, t tracks.Track) error) error {
	handleErr := func(err error) error {
		return fmt.Errorf("get track by link: %w", err)
	}
	const defaultCount = 10
	var cursor uint64 = 0
	for {
		links, newCursor, err := r.client.SScan(ctx, "trackprimarylinks", cursor, "", defaultCount).Result()
		if err != nil {
			return handleErr(err)
		}
		for _, link := range links {
			rawTrackMsg, err := r.client.HGet(ctx, "tracks", link).Bytes()
			if err != nil {
				return handleErr(err)
			}
			var trackMsg protos.Track
			if err := proto.Unmarshal(rawTrackMsg, &trackMsg); err != nil {
				return handleErr(err)
			}
			trk := msgToTrack(&trackMsg)
			if err := run(ctx, trk); err != nil {
				return handleErr(err)
			}
			cursor = newCursor
		}
	}
}
