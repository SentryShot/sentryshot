// Package rec2mp4 is a CLI utility that converts recordings into mp4 files.
package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"nvr/pkg/storage"
	"os"
	"path/filepath"
)

const usage = `convert recordings into mp4 files
example: rec2mp4 ./storage/recordings"`

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error { //nolint:funlen
	args := os.Args
	if len(args) != 2 {
		fmt.Println(usage)
		return nil
	}

	var recordings []string

	path := args[1]

	walkFunc := func(path string, info fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("%v %w", path, err)
		}
		if info.IsDir() || len(path) < 5 {
			return nil
		}
		if path[len(path)-5:] != ".meta" {
			return nil
		}

		recording := path[:len(path)-5]

		_, err = os.Stat(recording + ".mdat")
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return fmt.Errorf("%v %w", path, err)
		}

		_, err = os.Stat(recording + ".mp4")
		if !errors.Is(err, os.ErrNotExist) {
			return nil
		}

		recordings = append(recordings, recording)
		return nil
	}
	err := filepath.WalkDir(path, walkFunc)
	if err != nil {
		return err
	}

	nRecordings := len(recordings)
	fmt.Printf("Found %v new recordings.\n", nRecordings)

	chResults := make(chan result, nRecordings)
	for _, recording := range recordings {
		go func(recording string) {
			chResults <- result{
				recording: recording,
				err:       convert(recording),
			}
		}(recording)
	}

	for i := 1; i <= nRecordings; i++ {
		result := <-chResults
		fmt.Printf("[%v/%v]", i, nRecordings)
		if result.err != nil {
			fmt.Printf("[ERR] %v %v\n", result.recording, err)
			continue
		}
		fmt.Printf("[OK] %v\n", result.recording+".mp4")
	}
	return nil
}

type result struct {
	recording string
	err       error
}

func convert(recording string) error {
	video, err := storage.NewVideoReader(recording, nil)
	if err != nil {
		return fmt.Errorf("create video reader: %w", err)
	}
	defer video.Close()

	file, err := os.OpenFile(recording+".mp4", os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	_, err = io.Copy(file, video)
	if err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	return nil
}
