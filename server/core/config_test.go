package core

import (
	"strings"
	"testing"
	"time"
)

const sampleConfig = `
listen: 127.0.0.1:9090
allowedHosts: [a.lan]
sources:
  - { id: prowlarr, type: prowlarr, url: http://p:9696, apiKey: ${TTB_TEST_KEY} }
engines:
  - { id: torbox, type: torbox, apiKey: k, removeAfterCopy: true }
storages:
  - { id: dl, type: path, label: DL, root: /tmp }
defaults: { engine: torbox, storage: dl }
limits: { jobs: 3, indexerTimeout: 5s }
`

func TestParseConfig(t *testing.T) {
	t.Setenv("TTB_TEST_KEY", "secret")
	c, err := ParseConfig([]byte(sampleConfig))
	if err != nil {
		t.Fatal(err)
	}
	if c.Sources[0]["apiKey"] != "secret" {
		t.Errorf("env expansion: %v", c.Sources[0]["apiKey"])
	}
	if c.Defaults.Mode != ModeCopy || c.Limits.Jobs != 3 || c.Limits.FilesPerJob != 2 {
		t.Errorf("defaults: %+v %+v", c.Defaults, c.Limits)
	}
	if c.Limits.IndexerTimeout != 5*time.Second || c.Limits.ResultTTL != 30*time.Minute {
		t.Errorf("durations: %+v", c.Limits)
	}
	if !BoolOpt(c.Engines[0], "removeAfterCopy") {
		t.Error("removeAfterCopy not read")
	}
}

func TestParseConfigErrors(t *testing.T) {
	cases := map[string]string{
		"unset env":    "sources: [{id: a, type: prowlarr, apiKey: ${TTB_UNSET_VAR_X}}]",
		"dup id":       "engines: [{id: a, type: torbox}]\nstorages: [{id: a, type: path}]",
		"missing type": "engines: [{id: a}]",
		"bad default":  "defaults: {engine: nope}",
		"bad mode":     "defaults: {mode: teleport}",
		"bad webhook":  "notify: {webhook: ntfy.sh/x}",
		"invalid yaml": "listen: [",
	}
	for name, raw := range cases {
		if _, err := ParseConfig([]byte(raw)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestBuildRegistryUnknownType(t *testing.T) {
	c, err := ParseConfig([]byte("engines: [{id: x, type: nosuch}]"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildRegistry(c); err == nil || !strings.Contains(err.Error(), "unknown type") {
		t.Errorf("err = %v", err)
	}
}
