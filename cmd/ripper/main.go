package main

import (
	"accu/cmd"
	"accu/drivers/channelfetcher"
	"accu/drivers/fetcher"
	"accu/drivers/repo"
	"accu/tracks/usecase"
	"context"
	"database/sql"
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
	sqliteName := cmd.DefaultSqliteName
	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s.sqlite?mode=rwc&cache=shared", sqliteName))
	if err != nil {
		return handleErr(err)
	}
	defer db.Close()
	r := repo.New(db)
	if err != nil {
		return handleErr(err)
	}
	if err := r.Create(); err != nil {
		return handleErr(err)
	}
	ucfg := usecase.Cfg{
		DownloadsRootDir: "downloads",
	}
	u := usecase.New(ucfg, rt, tlf, cf, r, l)
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)
		<-sigint
		cancel()
	}()
	if err := u.Rip(ctx); err != nil {
		return handleErr(err)
	}
	return nil
}
