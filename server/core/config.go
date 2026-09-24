package core

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// AdapterConfig is one entry of sources/engines/storages. Every key of the
// YAML mapping (including id and type) is handed to the adapter constructor.
type AdapterConfig map[string]any

// ID returns the adapter id declared in the config.
func (a AdapterConfig) ID() string { return StringOpt(a, "id") }

// Type returns the adapter type used to look up its constructor.
func (a AdapterConfig) Type() string { return StringOpt(a, "type") }

// Defaults preselects the download dialog.
type Defaults struct {
	Engine  string `yaml:"engine" json:"engine"`
	Storage string `yaml:"storage" json:"storage"`
	Mode    Mode   `yaml:"mode" json:"mode"`
}

// Limits bounds concurrency, timeouts and cache sizes.
type Limits struct {
	Jobs              int           `yaml:"jobs"`
	FilesPerJob       int           `yaml:"filesPerJob"`
	ResultsPerIndexer int           `yaml:"resultsPerIndexer"`
	IndexerTimeout    time.Duration `yaml:"indexerTimeout"`
	ResultTTL         time.Duration `yaml:"resultTTL"`
	ResultCap         int           `yaml:"resultCap"`
	JobHistory        int           `yaml:"jobHistory"`
}

// NotifyConfig holds the outgoing webhook.
type NotifyConfig struct {
	Webhook string `yaml:"webhook"`
	Lang    string `yaml:"lang"` // "en" (default) or "fr": language of the notification titles
}

// Config is the whole YAML file.
type Config struct {
	Listen       string          `yaml:"listen"`
	AllowedHosts []string        `yaml:"allowedHosts"`
	Token        string          `yaml:"token"`
	DataDir      string          `yaml:"dataDir"`
	Sources      []AdapterConfig `yaml:"sources"`
	Engines      []AdapterConfig `yaml:"engines"`
	Storages     []AdapterConfig `yaml:"storages"`
	Defaults     Defaults        `yaml:"defaults"`
	Limits       Limits          `yaml:"limits"`
	Notify       NotifyConfig    `yaml:"notify"`
}

var envRef = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// LoadConfig reads, env-expands, parses and validates a YAML config file.
func LoadConfig(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseConfig(raw)
}

// ParseConfig parses and validates YAML bytes. ${VAR} references are replaced
// by the environment; an unset variable is an error.
func ParseConfig(raw []byte) (*Config, error) {
	var missing []string
	expanded := envRef.ReplaceAllStringFunc(string(raw), func(m string) string {
		name := m[2 : len(m)-1]
		v, ok := os.LookupEnv(name)
		if !ok {
			missing = append(missing, name)
		}
		return v
	})
	if len(missing) > 0 {
		return nil, fmt.Errorf("config: unset environment variable(s): %s", strings.Join(missing, ", "))
	}
	var c Config
	if err := yaml.Unmarshal([]byte(expanded), &c); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	c.applyDefaults()
	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	return &c, nil
}

func (c *Config) applyDefaults() {
	if c.Listen == "" {
		c.Listen = "0.0.0.0:8080"
	}
	if c.DataDir == "" {
		c.DataDir = "data"
	}
	if c.Defaults.Mode == "" {
		c.Defaults.Mode = ModeCopy
	}
	l := &c.Limits
	if l.Jobs <= 0 {
		l.Jobs = 2
	}
	if l.FilesPerJob <= 0 {
		l.FilesPerJob = 2
	}
	if l.ResultsPerIndexer <= 0 {
		l.ResultsPerIndexer = 100
	}
	if l.IndexerTimeout <= 0 {
		l.IndexerTimeout = 20 * time.Second
	}
	if l.ResultTTL <= 0 {
		l.ResultTTL = 30 * time.Minute
	}
	if l.ResultCap <= 0 {
		l.ResultCap = 20000
	}
	if l.JobHistory <= 0 {
		l.JobHistory = 200
	}
}

func (c *Config) validate() error {
	ids := map[string]string{}
	check := func(kind string, list []AdapterConfig) error {
		for i, a := range list {
			if a.ID() == "" {
				return fmt.Errorf("%s[%d]: missing id", kind, i)
			}
			if a.Type() == "" {
				return fmt.Errorf("%s %q: missing type", kind, a.ID())
			}
			if prev, dup := ids[a.ID()]; dup {
				return fmt.Errorf("%s %q: id already used by a %s", kind, a.ID(), prev)
			}
			ids[a.ID()] = kind
		}
		return nil
	}
	if err := check("source", c.Sources); err != nil {
		return err
	}
	if err := check("engine", c.Engines); err != nil {
		return err
	}
	if err := check("storage", c.Storages); err != nil {
		return err
	}
	if c.Defaults.Engine != "" && ids[c.Defaults.Engine] != "engine" {
		return fmt.Errorf("defaults.engine %q is not a configured engine", c.Defaults.Engine)
	}
	if c.Defaults.Storage != "" && ids[c.Defaults.Storage] != "storage" {
		return fmt.Errorf("defaults.storage %q is not a configured storage", c.Defaults.Storage)
	}
	switch c.Defaults.Mode {
	case ModeCopy, ModeAdopt, ModeLinks:
	default:
		return fmt.Errorf("defaults.mode %q: want copy, adopt or links", c.Defaults.Mode)
	}
	if c.Notify.Webhook != "" && !strings.HasPrefix(c.Notify.Webhook, "http://") && !strings.HasPrefix(c.Notify.Webhook, "https://") {
		return fmt.Errorf("notify.webhook must be an http(s) URL")
	}
	switch c.Notify.Lang {
	case "":
		c.Notify.Lang = "en"
	case "en", "fr":
	default:
		return fmt.Errorf("notify.lang must be en or fr")
	}
	return nil
}

// StringOpt reads a string option from an adapter config ("" when absent).
func StringOpt(cfg map[string]any, key string) string {
	switch v := cfg[key].(type) {
	case string:
		return v
	case int, int64, float64, bool:
		return fmt.Sprint(v)
	}
	return ""
}

// BoolOpt reads a bool option from an adapter config (false when absent).
func BoolOpt(cfg map[string]any, key string) bool {
	switch v := cfg[key].(type) {
	case bool:
		return v
	case string:
		return v == "true" || v == "1" || v == "yes"
	}
	return false
}

// StringMapOpt reads a string to string mapping option (nil when absent).
func StringMapOpt(cfg map[string]any, key string) map[string]string {
	out := map[string]string{}
	switch m := cfg[key].(type) {
	case map[string]any:
		for k, v := range m {
			out[k] = StringOpt(map[string]any{"v": v}, "v")
		}
	case map[string]string:
		for k, v := range m {
			out[k] = v
		}
	default:
		return nil
	}
	return out
}
