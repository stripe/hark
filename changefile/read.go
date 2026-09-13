package changefile

import (
	"context"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/afero"
	"golang.org/x/sync/errgroup"
)

// controls how [ReadAll] and [ReadEvery] walk and parse a directory tree.
type ReadOptions struct {
	// caps how many files are parsed concurrently; defaults to the number of available CPUs
	Workers int
}

// NumWorkers resolves [ReadOptions.Workers] to a concrete count, so that everything
// bounding a worker pool over changefiles agrees on what an unset value means.
func (o ReadOptions) NumWorkers() int {
	if o.Workers <= 0 {
		return runtime.NumCPU()
	}
	return o.Workers
}

// ReadResult the output of every (attempted) operation in [ReadAll]. Exactly one of Changefile and Err is set.
type ReadResult struct {
	// the file this result is about; always set
	Path string
	// the parsed file, or `nil` when [ReadResult.Err] is set.
	Changefile *Changefile
	// why this file could not be read or parsed, or `nil`.
	Err error
}

// ReadAll recursively discovers every changefile under `root` and parses them in
// parallel using a bounded worker pool, in sorted path order. Non `.change.md` files are ignored.
//
// Returns a [ReadResult] for every changefile, regardless of if parsing was successful. It only errors if the recursion itself is unsuccessful.
//
// Callers that want to stop on the first failure should use [ReadEvery] instead.
func ReadAll(ctx context.Context, fs afero.Fs, root string, opts ReadOptions) ([]ReadResult, error) {
	paths, err := GetAllPaths(fs, root)
	if err != nil {
		return nil, err
	}

	results := make([]ReadResult, len(paths))

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(opts.NumWorkers())

	for i, p := range paths {
		g.Go(func() error {
			if err := gCtx.Err(); err != nil {
				return err
			}

			cf, err := ReadFile(fs, p)
			// each goroutine only writes to a specific index, so there's no contention
			results[i] = ReadResult{Path: p, Changefile: cf, Err: err}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return results, nil
}

// ReadEvery is [ReadAll] for callers who need every available changefile to be structurally valid. It fails on the first file that could not be read or parsed.
func ReadEvery(ctx context.Context, fs afero.Fs, root string, opts ReadOptions) ([]*Changefile, error) {
	results, err := ReadAll(ctx, fs, root, opts)
	if err != nil {
		return nil, err
	}

	changefiles := make([]*Changefile, len(results))
	for i, r := range results {
		if r.Err != nil {
			return nil, r.Err
		}
		changefiles[i] = r.Changefile
	}

	return changefiles, nil
}

// GetAllPaths recursively lists the paths of every changefile under `root` in sorted order.
func GetAllPaths(fs afero.Fs, root string) ([]string, error) {
	var paths []string

	err := afero.Walk(fs, root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, Extension) {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return paths, nil
}
