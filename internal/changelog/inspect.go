package changelog

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/stripe/hark/changefile"
)

// Inspection is the machine-readable result for one explicitly named changefile.
// Path preserves the spelling supplied by the caller.
type Inspection struct {
	Path        string `json:"path"`
	SemverLevel string `json:"semver_level"`
}

// Inspect reads each requested changefile and writes its effective semver level as
// one JSON array. It reads all paths before writing so parse and read failures never
// leave partial JSON on the output stream.
func Inspect(ctx context.Context, opts Options, paths []string) error {
	opts = opts.withDefaults()
	results := make([]Inspection, 0, len(paths))

	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return err
		}

		cf, err := changefile.ReadFile(opts.Fs, path)
		if err != nil {
			return fmt.Errorf("inspecting %s: %w", path, err)
		}
		results = append(results, Inspection{Path: path, SemverLevel: cf.Level()})
	}

	return json.NewEncoder(opts.Out).Encode(results)
}
