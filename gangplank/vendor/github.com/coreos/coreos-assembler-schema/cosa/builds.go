package cosa

import (
	"bytes"
	"encoding/json"
	"io"
	"path/filepath"

	"github.com/pkg/errors"
)

const (
	// CosaBuildsJSON is the COSA build.json file name
	CosaBuildsJSON = "builds.json"
)

var (
	// ErrNoBuildsFound is thrown when a build is missing
	ErrNoBuildsFound = errors.New("no COSA builds found")
)

// build represents the build struct in a buildJSON
type build struct {
	ID     string   `json:"id"`
	Arches []string `json:"arches"`
}

// buildsJSON represents the JSON that records the builds
// TODO: this should be generated by a schema
type buildsJSON struct {
	SchemaVersion string  `json:"schema-version"`
	Builds        []build `json:"builds"`
	TimeStamp     string  `json:"timestamp"`
}

func getBuilds(dir string) (*buildsJSON, error) {
	path := filepath.Join(dir, CosaBuildsJSON)
	f, err := Open(path)
	if err != nil {
		return nil, ErrNoBuildsFound
	}
	d := []byte{}
	bufD := bytes.NewBuffer(d)
	if _, err := io.Copy(bufD, f); err != nil {
		return nil, err
	}
	b := &buildsJSON{}
	if err := json.Unmarshal(bufD.Bytes(), b); err != nil {
		return nil, err
	}
	return b, nil
}

// getLatest returns the latest build for the arch.
func (b *buildsJSON) getLatest(arch string) (string, bool) {
	for _, b := range b.Builds {
		for _, a := range b.Arches {
			if a == arch {
				return b.ID, true
			}
		}
	}
	return "", false
}
