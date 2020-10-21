package main

import (
	"accu/drivers/channelfetcher"
	"accu/drivers/fetcher"
	"accu/drivers/repo"
	"accu/tracks/usecase"
	"context"
	"database/sql"
	"net/http"
	"os"
	"os/signal"

	_ "github.com/mattn/go-sqlite3"
)

const accuURI = "https://www.accuradio.com/playlist/json/"

const defaultCategoryURI = "https://www.accuradio.com/indie-rock/"

func mustBeNil(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	rt := &http.Transport{}
	tfCfg := fetcher.Cfg{
		BaseURI: accuURI,
	}
	tlf := fetcher.NewTrackListFetcher(rt, tfCfg)
	cfCfg := channelfetcher.Cfg{
		BaseURI: defaultCategoryURI,
	}
	if len(os.Args) == 2 {
		cfCfg.BaseURI = os.Args[1]
	}
	cf := channelfetcher.NewChannelFetcher(rt, cfCfg)
	db, err := sql.Open("sqlite3", "file:tracks.sqlite?mode=rwc&cache=shared")
	mustBeNil(err)
	defer db.Close()
	r := repo.New(db)
	mustBeNil(r.Create())
	ucfg := usecase.Cfg{
		DownloadsRootDir: "downloads",
	}
	u := usecase.New(ucfg, rt, tlf, cf, r)
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)
		<-sigint
		cancel()
	}()
	mustBeNil(u.Save(ctx))
}
