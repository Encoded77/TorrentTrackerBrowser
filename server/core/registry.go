package core

import (
	"fmt"
	"sort"
)

// Adapter constructors, keyed by config "type". Adapter packages register
// themselves from init(); main imports them for side effects.
var (
	sourceCtors  = map[string]func(cfg map[string]any) (Source, error){}
	engineCtors  = map[string]func(cfg map[string]any) (Engine, error){}
	storageCtors = map[string]func(cfg map[string]any) (Storage, error){}
)

// RegisterSource registers a Source constructor for a config type.
func RegisterSource(typ string, ctor func(cfg map[string]any) (Source, error)) {
	sourceCtors[typ] = ctor
}

// RegisterEngine registers an Engine constructor for a config type.
func RegisterEngine(typ string, ctor func(cfg map[string]any) (Engine, error)) {
	engineCtors[typ] = ctor
}

// RegisterStorage registers a Storage constructor for a config type.
func RegisterStorage(typ string, ctor func(cfg map[string]any) (Storage, error)) {
	storageCtors[typ] = ctor
}

// RegisteredTypes lists the known adapter types (for error messages).
func RegisteredTypes() (sources, engines, storages []string) {
	for k := range sourceCtors {
		sources = append(sources, k)
	}
	for k := range engineCtors {
		engines = append(engines, k)
	}
	for k := range storageCtors {
		storages = append(storages, k)
	}
	sort.Strings(sources)
	sort.Strings(engines)
	sort.Strings(storages)
	return
}

// EngineOptions are the engine settings the job runner reads itself.
type EngineOptions struct {
	RemoveAfterCopy bool
}

// Registry holds the built adapters in config order.
type Registry struct {
	Sources  []Source
	Engines  []Engine
	Storages []Storage

	sources    map[string]Source
	engines    map[string]Engine
	storages   map[string]Storage
	engineOpts map[string]EngineOptions
}

// NewRegistry returns an empty registry (tests fill it with AddEngine and co).
func NewRegistry() *Registry {
	return &Registry{
		sources:    map[string]Source{},
		engines:    map[string]Engine{},
		storages:   map[string]Storage{},
		engineOpts: map[string]EngineOptions{},
	}
}

// BuildRegistry instantiates every adapter declared in the config. An unknown
// type or a failing constructor is a startup error.
func BuildRegistry(c *Config) (*Registry, error) {
	r := NewRegistry()
	for _, cfg := range c.Sources {
		ctor, ok := sourceCtors[cfg.Type()]
		if !ok {
			return nil, fmt.Errorf("source %q: unknown type %q", cfg.ID(), cfg.Type())
		}
		s, err := ctor(cfg)
		if err != nil {
			return nil, fmt.Errorf("source %q: %w", cfg.ID(), err)
		}
		r.AddSource(s)
	}
	for _, cfg := range c.Engines {
		ctor, ok := engineCtors[cfg.Type()]
		if !ok {
			return nil, fmt.Errorf("engine %q: unknown type %q", cfg.ID(), cfg.Type())
		}
		e, err := ctor(cfg)
		if err != nil {
			return nil, fmt.Errorf("engine %q: %w", cfg.ID(), err)
		}
		r.AddEngine(e, EngineOptions{RemoveAfterCopy: BoolOpt(cfg, "removeAfterCopy")})
	}
	for _, cfg := range c.Storages {
		ctor, ok := storageCtors[cfg.Type()]
		if !ok {
			return nil, fmt.Errorf("storage %q: unknown type %q", cfg.ID(), cfg.Type())
		}
		s, err := ctor(cfg)
		if err != nil {
			return nil, fmt.Errorf("storage %q: %w", cfg.ID(), err)
		}
		r.AddStorage(s)
	}
	return r, nil
}

// AddSource registers a built source.
func (r *Registry) AddSource(s Source) {
	r.Sources = append(r.Sources, s)
	r.sources[s.ID()] = s
}

// AddEngine registers a built engine with its runner-side options.
func (r *Registry) AddEngine(e Engine, o EngineOptions) {
	r.Engines = append(r.Engines, e)
	r.engines[e.ID()] = e
	r.engineOpts[e.ID()] = o
}

// AddStorage registers a built storage.
func (r *Registry) AddStorage(s Storage) {
	r.Storages = append(r.Storages, s)
	r.storages[s.ID()] = s
}

// Source looks up a source by id (nil when unknown).
func (r *Registry) Source(id string) Source { return r.sources[id] }

// Engine looks up an engine by id (nil when unknown).
func (r *Registry) Engine(id string) Engine { return r.engines[id] }

// Storage looks up a storage by id (nil when unknown).
func (r *Registry) Storage(id string) Storage { return r.storages[id] }

// EngineOptions returns the runner-side options of an engine.
func (r *Registry) EngineOptions(id string) EngineOptions { return r.engineOpts[id] }
