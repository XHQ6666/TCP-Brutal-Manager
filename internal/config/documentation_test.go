package config

import (
	"os"
	"strings"
	"testing"
)

func TestPublishedConfigurationExamples(t *testing.T) {
	b, err := os.ReadFile("../../docs/CONFIGURATION.md")
	if err != nil {
		t.Fatal(err)
	}
	_, body, ok := strings.Cut(string(b), "```json\n")
	if !ok {
		t.Fatal("configuration reference has no JSON example")
	}
	example, _, ok := strings.Cut(body, "\n```")
	if !ok {
		t.Fatal("unterminated JSON example")
	}
	sample, err := os.ReadFile("../../examples/config.json")
	if err != nil {
		t.Fatal(err)
	}
	for name, raw := range map[string][]byte{"reference": []byte(example), "package": sample} {
		t.Run(name, func(t *testing.T) {
			c, err := Parse(raw, "/etc/tcp-brutal-config/config.json")
			if err != nil {
				t.Fatal(err)
			}
			if len(c.Issues) != 0 {
				t.Fatalf("invalid published DNS rules: %+v", c.Issues)
			}
		})
	}
}
