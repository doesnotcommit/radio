package channelfetcher

import (
	"accu/tracks"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
)

type Cfg struct {
	BaseURI string
}

type ChannelFetcher struct {
	cfg Cfg
	c   *http.Client // pointer because http.DefaultClient is a pointer
}

func NewChannelFetcher(rt http.RoundTripper, cfg Cfg) ChannelFetcher {
	return ChannelFetcher{
		c: &http.Client{
			Transport: rt,
		},
		cfg: cfg,
	}
}

var channelRegexp = regexp.MustCompile(`data-id=["']([a-f\d]+)["']\s+data-oldid="\d+"\s+data-name=['"](.+?)['"]`)

func (cf ChannelFetcher) FetchChannels() ([]tracks.Channel, error) {
	handleErr := func(err error) ([]tracks.Channel, error) {
		return nil, fmt.Errorf("channel fetcher: fetch channels: %w", err)
	}
	resp, err := cf.c.Get(cf.cfg.BaseURI)
	if err != nil {
		return handleErr(err)
	}
	defer resp.Body.Close()
	rawPage, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return handleErr(err)
	}
	res := channelRegexp.FindAllStringSubmatch(string(rawPage), -1)
	uniqChannels := map[string]struct{}{}
	channels := make([]tracks.Channel, 0, len(res))
	for _, r := range res {
		if _, ok := uniqChannels[r[1]]; ok {
			continue
		}
		uniqChannels[r[1]] = struct{}{}
		channels = append(channels, tracks.Channel{
			Name:   r[2],
			DataId: r[1],
		})
	}
	return channels, nil
}
