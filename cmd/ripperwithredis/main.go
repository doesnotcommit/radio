package main

import (
	"accu/cmd"
	"accu/drivers/channelfetcher"
	"accu/drivers/fetcher"
	"accu/drivers/repo"
	"accu/tracks/usecase"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	l := log.Default()
	if err := run(l); err != nil {
		l.Println(err)
		os.Exit(1)
	}
}

func run(l *log.Logger) error {
	handleErr := func(err error) error {
		return fmt.Errorf("run: %w", err)
	}
	rt := &http.Transport{}
	tfCfg := fetcher.Cfg{
		BaseURI: cmd.DefaultAccuURI,
	}
	tlf := fetcher.NewTrackListFetcher(rt, tfCfg)
	cfCfg := channelfetcher.Cfg{
		BaseURI: cmd.DefaultCategoryURI,
	}
	if len(os.Args) == 2 {
		cfCfg.BaseURI = os.Args[1]
	}
	cf := channelfetcher.NewChannelFetcher(rt, cfCfg)
	ctx := context.Background()
	redisClient, cleanupRedis, err := repo.NewRedisClient(ctx, "localhost", 6379, l)
	if err != nil {
		return handleErr(err)
	}
	defer cleanupRedis()
	r := repo.NewRedis(redisClient, l)
	if err != nil {
		return handleErr(err)
	}
	ucfg := usecase.Cfg{
		DownloadsRootDir: "downloads", // TODO this is irrelevant, separate usecases
	}
	u := usecase.New(ucfg, rt, tlf, cf, r, l)
	ctx, cancelCtx := context.WithCancel(ctx)
	go func() {
		childCtx, cancelChildCtx := signal.NotifyContext(ctx, os.Interrupt)
		defer cancelChildCtx()
		<-childCtx.Done()
		cancelCtx()
	}()
	if err := u.Rip(ctx); err != nil {
		return handleErr(err)
	}
	return nil
}
