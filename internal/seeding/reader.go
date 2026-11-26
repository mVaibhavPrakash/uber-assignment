package seeding

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

type Summary struct {
	Areas   []Area `json:"areas"`
	Matched int64  `json:"observations"`
}

type Observation struct {
	Trip
	Weather   Weather   `json:"weather"`
	LocalDate time.Time `json:"local_date"`
	Season    int       `json:"season"`
	DayOfWeek int       `json:"day_of_week"`
	HourOfDay int       `json:"hour_of_day"`
}

type seedReader struct {
	file    *os.File
	decoder *json.Decoder
	areas   []Area
}

// The prepared file contains just areas and records, with no provenance envelope.
func openSeed(path string) (*seedReader, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	r := &seedReader{file: file, decoder: json.NewDecoder(file)}
	if err := r.readHeader(); err != nil {
		file.Close()
		return nil, err
	}
	return r, nil
}

func (r *seedReader) expect(want json.Token) error {
	token, err := r.decoder.Token()
	if err != nil {
		return err
	}
	if token != want {
		return fmt.Errorf("expected JSON %v, got %v", want, token)
	}
	return nil
}

func (r *seedReader) readHeader() error {
	if err := r.expect(json.Delim('{')); err != nil {
		return err
	}
	if err := r.expect("areas"); err != nil {
		return err
	}
	if err := r.decoder.Decode(&r.areas); err != nil {
		return err
	}
	if err := r.expect("records"); err != nil {
		return err
	}
	return r.expect(json.Delim('['))
}

func SeedHeader(dir string) (Summary, error) {
	r, err := openSeed(filepath.Join(dir, "seed.json"))
	if err != nil {
		return Summary{}, err
	}
	defer r.file.Close()
	return Summary{Areas: r.areas}, nil
}

// Streams the already-cleaned JSON into storage without repeating preparation checks.
func ScanSeed(ctx context.Context, path string, consume func(Observation) error) (s Summary, err error) {
	r, err := openSeed(path)
	if err != nil {
		return s, err
	}
	defer r.file.Close()
	s.Areas = r.areas
	for r.decoder.More() {
		if err := ctx.Err(); err != nil {
			return s, err
		}
		var row Observation
		if err := r.decoder.Decode(&row); err != nil {
			return s, err
		}
		if consume != nil {
			if err := consume(row); err != nil {
				return s, err
			}
		}
		s.Matched++
	}
	if err := r.finish(); err != nil {
		return s, err
	}
	return s, nil
}

// finish checks that the records array and JSON object end cleanly.
func (r *seedReader) finish() error {
	if err := r.expect(json.Delim(']')); err != nil {
		return err
	}
	if err := r.expect(json.Delim('}')); err != nil {
		return err
	}
	var trailing any
	if err := r.decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("unexpected trailing JSON: %v", err)
	}
	return nil
}
