package main

import (
	"testing"

	network "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v8"
)

type testInputAzure struct {
	cfg *azureConfig
	msg string
}

func getValidAzureConfig() *azureConfig {
	upstreams := []azureUpstream{
		{
			Name:       "backend1",
			VMScaleSet: "backend-group",
			Port:       80,
			Kind:       "http",
		},
	}
	cfg := azureConfig{
		SubscriptionID:    "subscription_id",
		ResourceGroupName: "resource_group_name",
		Upstreams:         upstreams,
	}

	return &cfg
}

func getInvalidAzureConfigInput() []*testInputAzure {
	var input []*testInputAzure

	invalidSubscriptionCfg := getValidAzureConfig()
	invalidSubscriptionCfg.SubscriptionID = ""
	input = append(input, &testInputAzure{invalidSubscriptionCfg, "invalid subscription id"})

	invalidResourceGroupNameCfg := getValidAzureConfig()
	invalidResourceGroupNameCfg.ResourceGroupName = ""
	input = append(input, &testInputAzure{invalidResourceGroupNameCfg, "invalid resource group name"})

	invalidMissingUpstreamsCfg := getValidAzureConfig()
	invalidMissingUpstreamsCfg.Upstreams = nil
	input = append(input, &testInputAzure{invalidMissingUpstreamsCfg, "no upstreams"})

	invalidUpstreamNameCfg := getValidAzureConfig()
	invalidUpstreamNameCfg.Upstreams[0].Name = ""
	input = append(input, &testInputAzure{invalidUpstreamNameCfg, "invalid name of the upstream"})

	invalidUpstreamVMMSetCfg := getValidAzureConfig()
	invalidUpstreamVMMSetCfg.Upstreams[0].VMScaleSet = ""
	input = append(input, &testInputAzure{invalidUpstreamVMMSetCfg, "invalid virtual_machine_scale_set of the upstream"})

	invalidUpstreamPortCfg := getValidAzureConfig()
	invalidUpstreamPortCfg.Upstreams[0].Port = 0
	input = append(input, &testInputAzure{invalidUpstreamPortCfg, "invalid port of the upstream"})

	invalidUpstreamKindCfg := getValidAzureConfig()
	invalidUpstreamKindCfg.Upstreams[0].Kind = ""
	input = append(input, &testInputAzure{invalidUpstreamKindCfg, "invalid kind of the upstream"})

	invalidUpstreamMaxConnsCfg := getValidAzureConfig()
	invalidUpstreamMaxConnsCfg.Upstreams[0].MaxConns = -10
	input = append(input, &testInputAzure{invalidUpstreamMaxConnsCfg, "invalid max_conns of the upstream"})

	invalidUpstreamMaxFailsCfg := getValidAzureConfig()
	invalidUpstreamMaxFailsCfg.Upstreams[0].MaxFails = -10
	input = append(input, &testInputAzure{invalidUpstreamMaxFailsCfg, "invalid max_fails of the upstream"})

	invalidUpstreamFailTimeoutCfg := getValidAzureConfig()
	invalidUpstreamFailTimeoutCfg.Upstreams[0].FailTimeout = "-10s"
	input = append(input, &testInputAzure{invalidUpstreamFailTimeoutCfg, "invalid fail_timeout of the upstream"})

	invalidUpstreamSlowStartCfg := getValidAzureConfig()
	invalidUpstreamSlowStartCfg.Upstreams[0].SlowStart = "-10s"
	input = append(input, &testInputAzure{invalidUpstreamSlowStartCfg, "invalid slow_start of the upstream"})

	return input
}

func TestValidateAzureConfigNotValid(t *testing.T) {
	t.Parallel()
	input := getInvalidAzureConfigInput()

	for _, item := range input {
		err := validateAzureConfig(item.cfg)
		if err == nil {
			t.Errorf("validateAzureConfig() didn't fail for the invalid config file with %v", item.msg)
		}
	}
}

func TestValidateAzureConfigValid(t *testing.T) {
	t.Parallel()
	cfg := getValidAzureConfig()

	err := validateAzureConfig(cfg)
	if err != nil {
		t.Errorf("validateAzureConfig() failed for the valid config: %v", err)
	}
}

func TestGetPrimaryIPFromInterfaceIPConfiguration(t *testing.T) {
	t.Parallel()
	primary := true
	address := "127.0.0.1"
	ipConfig := &network.InterfaceIPConfiguration{
		Properties: &network.InterfaceIPConfigurationPropertiesFormat{
			Primary:          &primary,
			PrivateIPAddress: &address,
		},
	}

	if getPrimaryIPFromInterfaceIPConfiguration(ipConfig) == "" {
		t.Errorf("getPrimaryIPFromInterfaceIPConfiguration() returned an empty ip, expected: %v", address)
	}
}

func TestGetPrimaryIPFromInterfaceIPConfigurationFail(t *testing.T) {
	t.Parallel()
	primaryFalse := false
	primaryTrue := true
	tests := []struct {
		ipConfig *network.InterfaceIPConfiguration
		msg      string
	}{
		{
			ipConfig: &network.InterfaceIPConfiguration{},
			msg:      "empty primary",
		},
		{
			ipConfig: &network.InterfaceIPConfiguration{
				Properties: &network.InterfaceIPConfigurationPropertiesFormat{
					Primary: &primaryFalse,
				},
			},
			msg: "not primary interface",
		},
		{
			ipConfig: &network.InterfaceIPConfiguration{
				Properties: nil,
			},
			msg: "no interface properties",
		},
		{
			ipConfig: &network.InterfaceIPConfiguration{
				Properties: &network.InterfaceIPConfigurationPropertiesFormat{
					Primary:          &primaryTrue,
					PrivateIPAddress: nil,
				},
			},
			msg: "no private ip address",
		},
	}

	for _, test := range tests {
		if getPrimaryIPFromInterfaceIPConfiguration(test.ipConfig) != "" {
			t.Errorf("getPrimaryIPFromInterfaceIPConfiguration() returned a non empty string for case: %v", test.msg)
		}
	}
}

func TestGetUpstreamsAzure(t *testing.T) {
	t.Parallel()
	cfg := getValidAzureConfig()
	upstreams := []azureUpstream{
		{
			Name:        "127.0.0.1",
			Port:        80,
			MaxFails:    1,
			MaxConns:    2,
			SlowStart:   "5s",
			FailTimeout: "10s",
		},
		{
			Name:        "127.0.0.2",
			Port:        80,
			MaxFails:    2,
			MaxConns:    3,
			SlowStart:   "6s",
			FailTimeout: "11s",
		},
	}
	cfg.Upstreams = upstreams
	c := AzureClient{config: cfg}

	ups := c.GetUpstreams()
	for _, u := range ups {
		found := false
		for _, cfgU := range cfg.Upstreams {
			if u.Name == cfgU.Name {
				if !areEqualUpstreamsAzure(cfgU, u) {
					t.Errorf("GetUpstreams() returned a wrong Upstream %+v for the configuration %+v", u, cfgU)
				}
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Upstream %+v not found in configuration.", u)
		}
	}
}

func areEqualUpstreamsAzure(u1 azureUpstream, u2 Upstream) bool {
	if u1.Port != u2.Port {
		return false
	}

	if u1.FailTimeout != u2.FailTimeout {
		return false
	}

	if u1.SlowStart != u2.SlowStart {
		return false
	}

	if u1.MaxConns != *u2.MaxConns {
		return false
	}

	if u1.MaxFails != *u2.MaxFails {
		return false
	}

	return true
}
