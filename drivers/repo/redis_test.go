package repo

import (
	"accu/drivers/repo/proto"
	"testing"
)

func TestRedis(t *testing.T) {
	m := proto.Track{
		Channel:       "chan",
		Artist:        "artist",
		Album:         "album",
		Title:         "title",
		Year:          2020,
		PrimaryLink:   "primaryLink",
		SecondaryLink: "secLink",
		Duration:      30,
	}
	t.Log(m.String())
}
