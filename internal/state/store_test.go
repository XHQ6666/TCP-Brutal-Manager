package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStore(t *testing.T) {
	dir := t.TempDir()
	_, bad, e := Load(dir)
	if e != nil || bad {
		t.Fatal(bad, e)
	}
	if e = Save(dir, Data{Version: 1, Generation: "test"}); e != nil {
		t.Fatal(e)
	}
	d, bad, e := Load(dir)
	if e != nil || bad || d.Generation != "test" {
		t.Fatal(d, bad, e)
	}
	os.WriteFile(filepath.Join(dir, "state.json"), []byte("broken"), 0600)
	_, bad, e = Load(dir)
	if e != nil || !bad {
		t.Fatal(bad, e)
	}
	os.Remove(filepath.Join(dir, "state.json"))
	_, bad, e = Load(dir)
	if e != nil || !bad {
		t.Fatal(bad, e)
	}
}
