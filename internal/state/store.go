package state

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/XHQ6666/TCP-Brutal-Manager/internal/route"
	"github.com/XHQ6666/TCP-Brutal-Manager/internal/rules"
	"os"
	"path/filepath"
)

type Record struct {
	Rule rules.Rule `json:"rule"`
}
type Attribute struct {
	Unresolved     bool        `json:"unresolved,omitempty"`
	Route          route.Route `json:"route"`
	AddedAlgorithm bool        `json:"added_algorithm"`
	Sources        []string    `json:"sources,omitempty"`
	ChangedLock    bool        `json:"changed_lock"`
}
type Data struct {
	Version    int         `json:"version"`
	Records    []Record    `json:"records"`
	Pending    []Record    `json:"pending,omitempty"`
	Attributes []Attribute `json:"attributes"`
	Recovery   string      `json:"recovery,omitempty"`
	Generation string      `json:"generation"`
}
type envelope struct {
	Data     json.RawMessage `json:"data"`
	Checksum string          `json:"checksum"`
}

func Load(dir string) (Data, bool, error) {
	d := Data{Version: 1}
	b, e := os.ReadFile(filepath.Join(dir, "state.json"))
	_, markerErr := os.Stat(filepath.Join(dir, "initialized"))
	if markerErr != nil && !os.IsNotExist(markerErr) {
		return d, false, markerErr
	}
	marked := markerErr == nil
	if os.IsNotExist(e) {
		return d, marked, nil
	}
	if e != nil {
		return d, false, e
	}
	var env envelope
	if e = json.Unmarshal(b, &env); e != nil {
		return d, marked, nil
	}
	var canonical bytes.Buffer
	if e = json.Compact(&canonical, env.Data); e != nil {
		return d, marked, nil
	}
	sum := sha256.Sum256(canonical.Bytes())
	if hex.EncodeToString(sum[:]) != env.Checksum {
		return d, marked, nil
	}
	if e = json.Unmarshal(env.Data, &d); e != nil || (d.Version != 1 && d.Version != 2) {
		return Data{Version: 1}, marked, nil
	}
	return d, false, nil
}
func Save(dir string, d Data) error {
	d.Version = 2
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	if old, e := os.ReadFile(filepath.Join(dir, "state.json")); e == nil {
		var env envelope
		var previous Data
		if json.Unmarshal(old, &env) == nil && json.Unmarshal(env.Data, &previous) == nil && previous.Version == 1 {
			path := filepath.Join(dir, "state.v1.backup.json")
			if _, err := os.Stat(path); os.IsNotExist(err) {
				if err = atomic(dir, "state.v1.backup.json", old); err != nil {
					return err
				}
			}
		}
	}
	b, e := json.Marshal(d)
	if e != nil {
		return e
	}
	sum := sha256.Sum256(b)
	out, e := json.MarshalIndent(envelope{b, hex.EncodeToString(sum[:])}, "", "  ")
	if e != nil {
		return e
	}
	if e = atomic(dir, "state.json", out); e != nil {
		return e
	}
	return atomic(dir, "initialized", []byte("1\n"))
}
func atomic(dir, name string, b []byte) error {
	f, e := os.CreateTemp(dir, ".state-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if e = f.Chmod(0600); e != nil {
		f.Close()
		return e
	}
	if _, e = f.Write(b); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	if e = os.Rename(f.Name(), filepath.Join(dir, name)); e != nil {
		return e
	}
	fd, e := os.Open(dir)
	if e != nil {
		return e
	}
	defer fd.Close()
	return fd.Sync()
}
func Generation(b []byte) string { s := sha256.Sum256(b); return fmt.Sprintf("%x", s) }
