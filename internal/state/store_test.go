package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStore(t *testing.T) {
	firstInstall := t.TempDir()
	if e := os.WriteFile(filepath.Join(firstInstall, "state.json"), []byte("broken"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, recovery, e := Load(firstInstall); e != nil || recovery {
		t.Fatalf("unmarked damaged state must be treated as first install: recovery=%t err=%v", recovery, e)
	}

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

func TestVersionOneMigrationBackup(t *testing.T) {
	dir := t.TempDir()
	b := []byte(`{"version":1,"records":[],"attributes":[],"generation":"old"}`)
	sum := sha256.Sum256(b)
	out, _ := json.Marshal(envelope{Data: b, Checksum: hex.EncodeToString(sum[:])})
	if e := os.WriteFile(filepath.Join(dir, "state.json"), out, 0600); e != nil {
		t.Fatal(e)
	}
	d, bad, e := Load(dir)
	if e != nil || bad || d.Version != 1 {
		t.Fatal(d, bad, e)
	}
	if e = Save(dir, d); e != nil {
		t.Fatal(e)
	}
	saved, e := os.ReadFile(filepath.Join(dir, "state.v1.backup.json"))
	if e != nil || string(saved) != string(out) {
		t.Fatal("missing original backup", e)
	}
	d, bad, e = Load(dir)
	if e != nil || bad || d.Version != 2 {
		t.Fatal(d, bad, e)
	}
}
