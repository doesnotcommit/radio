package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"

	"accu/tracks"
)

type Sqlite struct {
	sync.RWMutex
	db *sql.DB
}

func NewSqlite(db *sql.DB) *Sqlite {
	return &Sqlite{
		db: db,
	}
}

func (s *Sqlite) Create() error {
	defer s.lock()()
	handleErr := func(err error) error {
		return fmt.Errorf("create sqlite db: %w", err)
	}
	qs := [...]string{
		`PRAGMA foreign_keys = 1`,
		`CREATE TABLE IF NOT EXISTS track (
			id INTEGER PRIMARY KEY,
			channel TEXT NOT NULL REFERENCES channel (data_id),
			artist TEXT NOT NULL,
			album TEXT NOT NULL,
			title TEXT NOT NULL,
			duration INTEGER NOT NULL,
			year TEXT NOT NULL,
			primary_link TEXT NOT NULL UNIQUE,
			secondary_link TEXT NOT NULL UNIQUE
		)`,
		`CREATE TABLE IF NOT EXISTS channel (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			data_id TEXT NOT NULL UNIQUE
		)`,
	}
	for _, q := range qs {
		if _, err := s.db.Exec(q); err != nil {
			return handleErr(err)
		}
	}
	return nil

}

func (s *Sqlite) SaveTracks(ctx context.Context, trks ...tracks.Track) error {
	defer s.lock()()
	handleErr := func(err error) error {
		return fmt.Errorf("sqlite: save tracks: %w", err)
	}
	if err := s.tx(func(tx *sql.Tx) error {
		ctx = context.WithValue(ctx, ctxTxKey, tx)
		for _, t := range trks {
			if err := s.saveTrack(ctx, t); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return handleErr(err)
	}
	return nil
}

func (s *Sqlite) SaveChannels(ctx context.Context, chs ...tracks.Channel) error {
	defer s.lock()()
	handleErr := func(err error) error {
		return fmt.Errorf("sqlite: save channels: %w", err)
	}
	if err := s.tx(func(tx *sql.Tx) error {
		ctx = context.WithValue(ctx, ctxTxKey, tx)
		for _, ch := range chs {
			if err := s.saveChannel(ctx, ch); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return handleErr(err)
	}
	return nil
}

func (s *Sqlite) GetChannels(ctx context.Context) ([]tracks.Channel, error) {
	defer s.lock()()
	handleErr := func(err error) ([]tracks.Channel, error) {
		return nil, fmt.Errorf("sqlite: get channels: %w", err)
	}
	const q = "SELECT c.name, c.data_id FROM channel c"
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return handleErr(err)
	}
	defer rows.Close()
	var cc []tracks.Channel
	for rows.Next() {
		select {
		case <-ctx.Done():
			return handleErr(ctx.Err())
		default:
		}
		var c tracks.Channel
		if err := rows.Scan(&c.Name, &c.DataId); err != nil {
			return handleErr(err)
		}
		cc = append(cc, c)
	}
	if err := rows.Err(); err != nil {
		return handleErr(err)
	}
	return cc, nil
}

func (s *Sqlite) GetAllTracks(ctx context.Context, run func(ctx context.Context, t tracks.Track) error) error {
	defer s.rlock()()
	handleErr := func(err error) error {
		return fmt.Errorf("sqlite: get all tracks: %w", err)
	}
	const q = `SELECT
			c.name,
			t.artist,
			t.album,
			t.title,
			t.duration,
			t.year,
			t.primary_link,
			t.secondary_link
		FROM track t
		JOIN channel c ON c.data_id = t.channel`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return handleErr(err)
	}
	defer rows.Close()
	for rows.Next() {
		select {
		case <-ctx.Done():
			return handleErr(ctx.Err())
		default:
		}
		var t tracks.Track
		if err := rows.Scan(
			&t.Channel, &t.Artist, &t.Album,
			&t.Title, &t.Duration, &t.Year,
			&t.PrimaryLink, &t.SecondaryLink,
		); err != nil {
			return handleErr(err)
		}
		if err := run(ctx, t); err != nil {
			return handleErr(err)
		}
	}
	if err := rows.Err(); err != nil {
		return handleErr(err)
	}
	return nil
}

func (s *Sqlite) saveChannel(ctx context.Context, ch tracks.Channel) error {
	handleErr := func(err error) error {
		return fmt.Errorf("sqlite: save channel: %w", err)
	}
	const q = "INSERT INTO channel (name, data_id) VALUES ($1, $2) ON CONFLICT DO NOTHING"
	if _, err := s.getExecer(ctx).ExecContext(ctx, q, ch.Name, ch.DataId); err != nil {
		return handleErr(err)
	}
	return nil
}

func (s *Sqlite) saveTrack(ctx context.Context, track tracks.Track) error {
	handleErr := func(err error) error {
		return fmt.Errorf("sqlite: save track: %w", err)
	}
	const q = `
		INSERT INTO track (
			channel, artist, album,
			title, duration, year,
			primary_link, secondary_link
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT DO NOTHING`
	if _, err := s.getExecer(ctx).ExecContext(ctx, q, track.Channel, track.Artist, track.Album, track.Title, track.Duration, track.Year, track.PrimaryLink, track.SecondaryLink); err != nil {
		return handleErr(err)
	}
	return nil
}

func (s *Sqlite) GetTrackByLink(ctx context.Context, link string) (tracks.Track, error) {
	defer s.rlock()()
	handleErr := func(err error) (tracks.Track, error) {
		return tracks.Track{}, fmt.Errorf("sqlite: get track: %w", err)
	}
	const q = `
		SELECT
			channel, artist, album,
			title, duration, year,
			primary_link, secondary_link
		FROM track
		WHERE primary_link = $1
		OR secondary_link = $1`
	var t tracks.Track
	if err := s.db.QueryRowContext(ctx, q, link).Scan(
		&t.Channel, &t.Artist, &t.Album,
		&t.Title, &t.Duration, &t.Year,
		&t.PrimaryLink, &t.SecondaryLink,
	); errors.Is(err, sql.ErrNoRows) {
		return handleErr(tracks.ErrNotFound)
	} else if err != nil {
		return handleErr(err)
	}
	return t, nil
}

func (s *Sqlite) rlock() func() {
	s.RLock()
	return func() {
		s.RUnlock()
	}
}

func (s *Sqlite) lock() func() {
	s.Lock()
	return func() {
		s.Unlock()
	}
}

func (s *Sqlite) tx(run func(tx *sql.Tx) error) error {
	handleErr := func(err error) error {
		return fmt.Errorf("sqlite tx: %w", err)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return handleErr(err)
	}
	if err := run(tx); err != nil {
		if er := tx.Rollback(); er != nil {
			return handleErr(fmt.Errorf("rollback: %v: %w", er, err))
		}
		return handleErr(err)
	}
	if er := tx.Commit(); er != nil {
		return handleErr(er)
	}
	return nil
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

type txKey int

const ctxTxKey txKey = 0

func (s *Sqlite) getExecer(ctx context.Context) execer {
	tx, ok := ctx.Value(ctxTxKey).(*sql.Tx)
	if ok {
		return tx
	}
	return s.db
}
