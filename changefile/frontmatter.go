package changefile

import (
	"bytes"
	"errors"
)

var (
	ErrNoFrontmatter       = errors.New("missing opening frontmatter delimiter (---)")
	ErrUnclosedFrontmatter = errors.New("missing closing frontmatter delimiter (---)")
)

var delimiter = []byte("---\n")

// SplitFrontmatter extracts YAML frontmatter and the remaining body from raw file content.
// It expects the file to begin with "---\n", followed by YAML, followed by another "---\n" line.
// Everything after the closing delimiter is returned as the body.
func SplitFrontmatter(content []byte) (yaml []byte, body string, err error) {
	if !bytes.HasPrefix(content, delimiter) {
		return nil, "", ErrNoFrontmatter
	}

	rest := content[len(delimiter):]
	end := bytes.Index(rest, delimiter)
	if end == -1 {
		// Also handle "---" at EOF without trailing newline
		if bytes.HasSuffix(rest, []byte("---")) {
			end = len(rest) - 3
			return rest[:end], "", nil
		}
		return nil, "", ErrUnclosedFrontmatter
	}

	yaml = rest[:end]
	body = string(rest[end+len(delimiter):])
	return yaml, body, nil
}
