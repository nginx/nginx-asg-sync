package main

import (
	"errors"
	"fmt"
	"time"

	yaml "gopkg.in/yaml.v3"
)

// commonConfig stores the configuration parameters common to all providers.
type commonConfig struct {
	CustomHeaders map[string]string `yaml:"custom_headers,omitempty"`
	APIEndpoint   string            `yaml:"api_endpoint"`
	CloudProvider string            `yaml:"cloud_provider"`
	SyncInterval  time.Duration     `yaml:"sync_interval"`
}

func parseCommonConfig(data []byte) (*commonConfig, error) {
	cfg := &commonConfig{}
	err := yaml.Unmarshal(data, cfg)
	if err != nil {
		return nil, fmt.Errorf("couldn't unmarshal common config: %w", err)
	}

	err = validateCommonConfig(cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func validateCommonConfig(cfg *commonConfig) error {
	if cfg.APIEndpoint == "" {
		return fmt.Errorf(errorMsgFormat, "api_endpoint")
	}

	if cfg.SyncInterval <= 0 {
		return errors.New(intervalErrorMsg)
	}

	if cfg.CloudProvider == "" {
		cfg.CloudProvider = defaultCloudProvider
	}

	if !validateCloudProvider(cfg.CloudProvider) {
		return fmt.Errorf(cloudProviderErrorMsg, cfg.CloudProvider)
	}

	return nil
}

// Upstream is the cloud agnostic representation of an Upstream (eg, common fields for every cloud provider).
type Upstream struct {
	MaxConns     *int
	MaxFails     *int
	Name         string
	ScalingGroup string
	Kind         string
	FailTimeout  string
	SlowStart    string
	Port         int
	InService    bool
}
