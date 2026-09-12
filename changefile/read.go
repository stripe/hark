package changefile

import (
	"context"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/afero"
	"golang.org/x/sync/errgroup"
)

// affects how [ReadAll] walks and parses a directory tree.
type ReadOptions struct {
	// caps how many files are parsed concurrently; defaults to the number of available CPUs
	Workers int
}

// Recursively discover every changefile under `root` and parse them in parallel using a bounded worker pool. It fails on the first file that cannot be read or parsed.
func ReadAll(ctx context.Context, fs afero.Fs, root string, opts ReadOptions) ([]*Changefile, error) {
	workers := opts.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	paths, err := FindAll(fs, root)
	if err != nil {
		return nil, err
	}

	results := make([]*Changefile, len(paths))

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(workers)

	for i, p := range paths {
		g.Go(func() error {
			if err := gCtx.Err(); err != nil {
				return err
			}

			cf, err := ReadFile(fs, p)
			if err != nil {
				return err
			}

			// each goroutine only writes to a specific index, so there's no contention
			results[i] = cf
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return results, nil
}

// FindAll recursively lists the paths of every changefile under `root` in sorted order.
func FindAll(fs afero.Fs, root string) ([]string, error) {
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
