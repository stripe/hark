package changefile

import (
	"bytes"
	"errors"
)

var (
	ErrNoFrontmatter       = errors.New("missing opening frontmatter delimiter (---)")
	ErrUnclosedFrontmatter = errors.New("missing closing frontmatter delimiter (---)")
)

var (
	openingDelimiter = []byte("---\n")
	delimiterLine    = []byte("---")
)

// SplitFrontmatter extracts YAML frontmatter and the remaining body from raw file content.
// It expects the file to begin with "---" followed by a newline, followed by YAML,
// followed by another "---" line. Unix and Windows (CRLF) newlines are accepted.
// Everything after the closing delimiter is returned as the body.
//
// Blank lines before the opening delimiter are ignored, since that's an easy typo to make by hand.
func SplitFrontmatter(content []byte) (yaml []byte, body string, err error) {
	content = bytes.TrimLeft(content, "\r\n")

	if !bytes.HasPrefix(content, openingDelimiter) && !bytes.HasPrefix(content, []byte("---\r\n")) {
		return nil, "", ErrNoFrontmatter
	}

	rest := content[len(openingDelimiter):]
	if bytes.HasPrefix(content, []byte("---\r\n")) {
		rest = content[len("---\r\n"):]
	}

	// Only a `---` on a line of its own closes the frontmatter, so a value that happens to
	// contain the delimiter can't cut the YAML short. The last line needs no trailing newline.
	for offset := 0; offset < len(rest); {
		line, next := rest[offset:], len(rest)
		if newline := bytes.IndexByte(line, '\n'); newline >= 0 {
			line, next = line[:newline], offset+newline+1
		}

		if bytes.Equal(bytes.TrimSuffix(line, []byte("\r")), delimiterLine) {
			return rest[:offset], string(rest[next:]), nil
		}
		offset = next
	}

	return nil, "", ErrUnclosedFrontmatter
}
