package spec

// Package spec provides helpers for loading and validating OCI bundle
// configuration files (config.json).  It parses the JSON into
// lightweight structures and extracts hook definitions for further
// processing.

import (
    "encoding/json"
    "fmt"
    "io/ioutil"
    "os"
)

// Bundle represents a minimal subset of the OCI config containing only
// the information we care about (hooks).
// See https://github.com/opencontainers/runtime-spec/blob/main/config.md for the full schema.
type Bundle struct {
    Hooks *Hooks `json:"hooks,omitempty"`
}

// Hooks represent the set of OCI hook arrays.  We care about all of
// them for tracing.
type Hooks struct {
    Prestart       []Hook `json:"prestart,omitempty"`
    CreateRuntime  []Hook `json:"createRuntime,omitempty"`
    CreateContainer []Hook `json:"createContainer,omitempty"`
    StartContainer []Hook `json:"startContainer,omitempty"`
    Poststart      []Hook `json:"poststart,omitempty"`
    Poststop       []Hook `json:"poststop,omitempty"`
}

// Hook describes a single OCI hook entry.
type Hook struct {
    Path string   `json:"path"`
    Args []string `json:"args,omitempty"`
    Env  []string `json:"env,omitempty"`
    Timeout int    `json:"timeout,omitempty"`
}

// LoadBundle parses the bundle's config.json and returns a Bundle.
func LoadBundle(configPath string) (*Bundle, error) {
    f, err := os.Open(configPath)
    if err != nil {
        return nil, fmt.Errorf("open config: %w", err)
    }
    defer f.Close()
    data, err := ioutil.ReadAll(f)
    if err != nil {
        return nil, fmt.Errorf("read config: %w", err)
    }
    var b Bundle
    if err := json.Unmarshal(data, &b); err != nil {
        return nil, fmt.Errorf("unmarshal config: %w", err)
    }
    return &b, nil
}