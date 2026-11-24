package changelog

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/stripe/hark/changefile"
	"github.com/stripe/hark/releases"
)

func Changes(ctx context.Context, opts Options, version string) error {
	opts = opts.withDefaults()
	if version != "" {
		ledger, err := releases.ReadFile(opts.Fs, opts.releasesPath())
		if err != nil {
			return err
		}
		if ledger.Find(version) == nil {
			return fmt.Errorf("unknown release %q in %s", version, opts.releasesPath())
		}
	}
	paths := []string{}
	if _, err := opts.Fs.Stat(opts.changesDir()); os.IsNotExist(err) {
		return json.NewEncoder(opts.Out).Encode(paths)
	} else if err != nil {
		return err
	}
	changes, err := changefile.ReadAllOrFail(ctx, opts.Fs, opts.changesDir(), opts.ReadOptions)
	if err != nil {
		return err
	}
	for _, change := range changes {
		if change.ReleasedInVersion != version {
			continue
		}
		path, err := filepath.Rel(opts.Root, change.SourcePath)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(path))
	}
	return json.NewEncoder(opts.Out).Encode(paths)
}
