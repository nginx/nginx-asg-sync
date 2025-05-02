package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v6"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v6"
	yaml "gopkg.in/yaml.v3"
)

// AzureClient allows you to get the list of IP addresses of VirtualMachines of a VirtualMachine Scale Set. It implements the CloudProvider interface.
type AzureClient struct {
	config      *azureConfig
	vMSSClient  *armcompute.VirtualMachineScaleSetsClient
	iFaceClient *armnetwork.InterfacesClient
}

// NewAzureClient creates an AzureClient.
func NewAzureClient(data []byte) (*AzureClient, error) {
	azureClient := &AzureClient{}
	cfg, err := parseAzureConfig(data)
	if err != nil {
		return nil, fmt.Errorf("error validating config: %w", err)
	}

	azureClient.config = cfg

	err = azureClient.configure()
	if err != nil {
		return nil, fmt.Errorf("error configuring Azure Client: %w", err)
	}

	return azureClient, nil
}

// parseAzureConfig parses and validates AzureClient config.
func parseAzureConfig(data []byte) (*azureConfig, error) {
	cfg := &azureConfig{}
	err := yaml.Unmarshal(data, cfg)
	if err != nil {
		return nil, fmt.Errorf("couldn't unmarshal Azure config: %w", err)
	}

	err = validateAzureConfig(cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func (client *AzureClient) listScaleSetsNetworkInterfaces(ctx context.Context, resourceGroupName, vmssName string) ([]*armnetwork.Interface, error) {
	var result []*armnetwork.Interface
	pager := client.iFaceClient.NewListVirtualMachineScaleSetNetworkInterfacesPager(resourceGroupName, vmssName, nil)
	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing network interfaces: %w", err)
		}
		result = append(result, resp.Value...)
	}
	return result, nil
}

// GetPrivateIPsForScalingGroup returns the list of IP addresses of instances of the Virtual Machine Scale Set.
func (client *AzureClient) GetPrivateIPsForScalingGroup(name string) ([]string, error) {
	var ips []string

	ctx := context.TODO()

	iFaces, err := client.listScaleSetsNetworkInterfaces(ctx, client.config.ResourceGroupName, name)
	if err != nil {
		return nil, err
	}

	for _, iFace := range iFaces {
		if iFace.Properties.VirtualMachine != nil && iFace.Properties.VirtualMachine.ID != nil && iFace.Properties.IPConfigurations != nil {
			for _, n := range iFace.Properties.IPConfigurations {
				ip := getPrimaryIPFromInterfaceIPConfiguration(n)
				if ip != "" {
					ips = append(ips, *n.Properties.PrivateIPAddress)
					break
				}
			}
		}
	}

	return ips, nil
}

func getPrimaryIPFromInterfaceIPConfiguration(ipConfig *armnetwork.InterfaceIPConfiguration) string {
	if ipConfig.Properties == nil {
		return ""
	}

	if ipConfig.Properties.Primary == nil {
		return ""
	}

	if !*ipConfig.Properties.Primary {
		return ""
	}

	if ipConfig.Properties.PrivateIPAddress == nil {
		return ""
	}

	return *ipConfig.Properties.PrivateIPAddress
}

// CheckIfScalingGroupExists checks if the Virtual Machine Scale Set exists.
func (client *AzureClient) CheckIfScalingGroupExists(name string) (bool, error) {
	ctx := context.TODO()
	expandType := armcompute.ExpandTypesForGetVMScaleSetsUserData
	vmss, err := client.vMSSClient.Get(ctx, client.config.ResourceGroupName, name, &armcompute.VirtualMachineScaleSetsClientGetOptions{Expand: &expandType})
	if err != nil {
		return false, fmt.Errorf("couldn't check if a Virtual Machine Scale Set exists: %w", err)
	}

	return vmss.ID != nil, nil
}

func (client *AzureClient) configure() error {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return fmt.Errorf("couldn't create authorizer: %w", err)
	}

	computeClientFactory, err := armcompute.NewClientFactory(client.config.SubscriptionID, cred, nil)
	if err != nil {
		return fmt.Errorf("couldn't create client factory: %w", err)
	}
	client.vMSSClient = computeClientFactory.NewVirtualMachineScaleSetsClient()

	iclient, err := armnetwork.NewInterfacesClient(client.config.SubscriptionID, cred, nil)
	if err != nil {
		return fmt.Errorf("couldn't create interfaces client: %w", err)
	}
	client.iFaceClient = iclient

	return nil
}

// GetUpstreams returns the Upstreams list.
func (client *AzureClient) GetUpstreams() []Upstream {
	upstreams := make([]Upstream, 0, len(client.config.Upstreams))
	for i := range len(client.config.Upstreams) {
		u := Upstream{
			Name:         client.config.Upstreams[i].Name,
			Port:         client.config.Upstreams[i].Port,
			Kind:         client.config.Upstreams[i].Kind,
			ScalingGroup: client.config.Upstreams[i].VMScaleSet,
			MaxConns:     &client.config.Upstreams[i].MaxConns,
			MaxFails:     &client.config.Upstreams[i].MaxFails,
			FailTimeout:  getFailTimeoutOrDefault(client.config.Upstreams[i].FailTimeout),
			SlowStart:    getSlowStartOrDefault(client.config.Upstreams[i].SlowStart),
		}
		upstreams = append(upstreams, u)
	}
	return upstreams
}

type azureConfig struct {
	SubscriptionID    string          `yaml:"subscription_id"`
	ResourceGroupName string          `yaml:"resource_group_name"`
	Upstreams         []azureUpstream `yaml:"upstreams"`
}

type azureUpstream struct {
	Name        string `yaml:"name"`
	VMScaleSet  string `yaml:"virtual_machine_scale_set"`
	Kind        string `yaml:"kind"`
	FailTimeout string `yaml:"fail_timeout"`
	SlowStart   string `yaml:"slow_start"`
	Port        int    `yaml:"port"`
	MaxConns    int    `yaml:"max_conns"`
	MaxFails    int    `yaml:"max_fails"`
}

func validateAzureConfig(cfg *azureConfig) error {
	if cfg.SubscriptionID == "" {
		return fmt.Errorf(errorMsgFormat, "subscription_id")
	}

	if cfg.ResourceGroupName == "" {
		return fmt.Errorf(errorMsgFormat, "resource_group_name")
	}

	if len(cfg.Upstreams) == 0 {
		return errors.New("there are no upstreams found in the config file")
	}

	for _, ups := range cfg.Upstreams {
		if ups.Name == "" {
			return errors.New(upstreamNameErrorMsg)
		}
		if ups.VMScaleSet == "" {
			return fmt.Errorf(upstreamErrorMsgFormat, "virtual_machine_scale_set", ups.Name)
		}
		if ups.Port == 0 {
			return fmt.Errorf(upstreamPortErrorMsgFormat, ups.Name)
		}
		if ups.Kind == "" || (ups.Kind != "http" && ups.Kind != "stream") {
			return fmt.Errorf(upstreamKindErrorMsgFormat, ups.Name)
		}
		if ups.MaxConns < 0 {
			return fmt.Errorf(upstreamMaxConnsErrorMsgFmt, ups.MaxConns)
		}
		if ups.MaxFails < 0 {
			return fmt.Errorf(upstreamMaxFailsErrorMsgFmt, ups.MaxFails)
		}
		if !isValidTime(ups.FailTimeout) {
			return fmt.Errorf(upstreamFailTimeoutErrorMsgFmt, ups.FailTimeout)
		}
		if !isValidTime(ups.SlowStart) {
			return fmt.Errorf(upstreamSlowStartErrorMsgFmt, ups.SlowStart)
		}
	}
	return nil
}
