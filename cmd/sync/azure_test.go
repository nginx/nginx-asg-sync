package main

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/network/mgmt/network"
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

	return input
}

func TestValidateAzureConfigNotValid(t *testing.T) {
	input := getInvalidAzureConfigInput()

	for _, item := range input {
		err := validateAzureConfig(item.cfg)
		if err == nil {
			t.Errorf("validateAzureConfig() didn't fail for the invalid config file with %v", item.msg)
		}
	}
}

func TestValidateAzureConfigValid(t *testing.T) {
	cfg := getValidAzureConfig()

	err := validateAzureConfig(cfg)
	if err != nil {
		t.Errorf("validateAzureConfig() failed for the valid config: %v", err)
	}
}

func TestGetPrimaryIPFromInterfaceIPConfiguration(t *testing.T) {
	primary := true
	address := "127.0.0.1"
	ipConfig := network.InterfaceIPConfiguration{
		InterfaceIPConfigurationPropertiesFormat: &network.InterfaceIPConfigurationPropertiesFormat{
			Primary:          &primary,
			PrivateIPAddress: &address,
		},
	}

	if getPrimaryIPFromInterfaceIPConfiguration(ipConfig) == "" {
		t.Errorf("getPrimaryIPFromInterfaceIPConfiguration() returned an empty ip, expected: %v", address)
	}
}

func TestGetPrimaryIPFromInterfaceIPConfigurationFail(t *testing.T) {
	primaryFalse := false
	primaryTrue := true
	var tests = []struct {
		ipConfig network.InterfaceIPConfiguration
		msg      string
	}{
		{
			ipConfig: network.InterfaceIPConfiguration{},
			msg:      "empty primary",
		},
		{
			ipConfig: network.InterfaceIPConfiguration{
				InterfaceIPConfigurationPropertiesFormat: &network.InterfaceIPConfigurationPropertiesFormat{
					Primary: &primaryFalse,
				},
			},
			msg: "not primary interface",
		},
		{
			ipConfig: network.InterfaceIPConfiguration{
				InterfaceIPConfigurationPropertiesFormat: nil,
			},
			msg: "no interface properties",
		},
		{
			ipConfig: network.InterfaceIPConfiguration{
				InterfaceIPConfigurationPropertiesFormat: &network.InterfaceIPConfigurationPropertiesFormat{
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
