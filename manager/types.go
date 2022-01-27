package main

import (
	"encoding/json"
)

type ConfigData struct {
	IngressClass string             `json:"-" mapstructure:"class"`
	Global       string             `json:"global,omitempty"`
	Defaults     string             `json:"defaults,omitempty"`
	Frontend     map[string]string  `json:"frontend,omitempty"`
	Backend      map[string]Backend `json:"backend,omitempty"`
}

type Backend struct {
	Port     string            `json:"port,omitempty"`
	Replicas uint64            `json:"replicas,omitempty"`
	Frontend map[string]string `json:"-"`
	Backend  string            `json:"backend,omitempty"`
}

// TODO: handle potential errors
func (c ConfigData) ToJsonBytes() []byte {
	b, _ := json.Marshal(c)
	return b
}
