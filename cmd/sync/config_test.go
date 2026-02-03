package main

import "testing"

var validYaml = []byte(`
cloud_provider: AWS
api_endpoint: http://127.0.0.1:8080/api
sync_interval: 5s
`)

type testInputCommon struct {
	cfg *commonConfig
	msg string
}

func getValidCommonConfig() *commonConfig {
	return &commonConfig{
		APIEndpoint:  "http://127.0.0.1:8080/api",
		SyncInterval: 1,
	}
}

func getInvalidCommonConfigInput() []*testInputCommon {
	input := make([]*testInputCommon, 0, 2)

	invalidAPIEndpointCfg := getValidCommonConfig()
	invalidAPIEndpointCfg.APIEndpoint = ""
	input = append(input, &testInputCommon{invalidAPIEndpointCfg, "invalid api_endpoint"})

	invalidSyncIntervalCfg := getValidCommonConfig()
	invalidSyncIntervalCfg.SyncInterval = 0
	input = append(input, &testInputCommon{invalidSyncIntervalCfg, "invalid sync_interval"})

	return input
}

func TestValidateCommonConfigNotValid(t *testing.T) {
	t.Parallel()
	input := getInvalidCommonConfigInput()

	for _, item := range input {
		err := validateCommonConfig(item.cfg)
		if err == nil {
			t.Errorf("validateCommonConfig() didn't fail for the invalid config file with %v", item.msg)
		}
	}
}

func TestValidateCommonConfigValid(t *testing.T) {
	t.Parallel()
	cfg := getValidCommonConfig()

	err := validateCommonConfig(cfg)
	if err != nil {
		t.Errorf("validateCommonConfig() failed for the valid config: %v", err)
	}
}

func TestParseCommonConfig(t *testing.T) {
	t.Parallel()
	_, err := parseCommonConfig(validYaml)
	if err != nil {
		t.Errorf("parseCommonConfig() failed for the valid config yaml: %v", string(validYaml))
	}
}
