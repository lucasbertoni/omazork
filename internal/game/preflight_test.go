package game_test

import (
	"io/fs"
	"testing"
	"testing/fstest"

	omazork "github.com/lucasbertoni/omazork"
)

// skewedData copies the embedded duration data and bumps one file's
// schemaVersion.
func skewedData(t *testing.T, file string) fs.FS {
	t.Helper()
	m := fstest.MapFS{}
	err := fs.WalkDir(omazork.Data, "data/actions", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := fs.ReadFile(omazork.Data, path)
		if err != nil {
			return err
		}
		m[path] = &fstest.MapFile{Data: raw}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	m["data/actions/"+file] = &fstest.MapFile{Data: []byte(`{"schemaVersion": 2}`)}
	return m
}
