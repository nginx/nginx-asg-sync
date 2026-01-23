package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v7"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v8"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"

	yaml "gopkg.in/yaml.v3"
)

type VMSSClient interface {
	Get(ctx context.Context, rg, name string, opts *armcompute.VirtualMachineScaleSetsClientGetOptions) (armcompute.VirtualMachineScaleSetsClientGetResponse, error)
}

type VMSSVMsClient interface {
	NewListPager(rg, name string, opts *armcompute.VirtualMachineScaleSetVMsClientListOptions) *runtime.Pager[armcompute.VirtualMachineScaleSetVMsClientListResponse]
}

type VMsClient interface {
	Get(ctx context.Context, rg, name string, opts *armcompute.VirtualMachinesClientGetOptions) (armcompute.VirtualMachinesClientGetResponse, error)
}

type InterfacesClient interface {
	NewListVirtualMachineScaleSetNetworkInterfacesPager(rg, vmss string, opts *armnetwork.InterfacesClientListVirtualMachineScaleSetNetworkInterfacesOptions) *runtime.Pager[armnetwork.InterfacesClientListVirtualMachineScaleSetNetworkInterfacesResponse]
	Get(ctx context.Context, rg, name string, opts *armnetwork.InterfacesClientGetOptions) (armnetwork.InterfacesClientGetResponse, error)
}

// AzureClient allows you to get the list of IP addresses of VirtualMachines of a VirtualMachine Scale Set. It implements the CloudProvider interface.
type AzureClient struct {
	config                 *azureConfig
	vMSSClient             VMSSClient
	vmssVmClient           VMSSVMsClient
	individualvmssVmClient VMsClient
	iFaceClient            InterfacesClient
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

// getInterfacesFromIndividualVMs gets network interfaces from individual VMs for flexible orchestration mode.
// This method is required because VMSS VM list API doesn't include network profile for flexible mode.
func (client *AzureClient) getInterfacesFromIndividualVMs(ctx context.Context, vmList []*armcompute.VirtualMachineScaleSetVM) ([]*armnetwork.Interface, error) {
	if len(vmList) == 0 {
		return []*armnetwork.Interface{}, nil
	}

	var interfaces []*armnetwork.Interface
	var errList []string

	for _, vm := range vmList {
		if vm.Name == nil {
			errList = append(errList, "VM with nil name found")
			continue
		}

		vmName := *vm.Name
		vmInterfaces, err := client.getNetworkInterfacesForVM(ctx, vmName)
		if err != nil {
			errList = append(errList, fmt.Sprintf("VM %s: %v", vmName, err))
			continue
		}

		interfaces = append(interfaces, vmInterfaces...)
	}

	if len(errList) > 0 {
		return nil, fmt.Errorf(
			"errors while getInterfacesFromIndividualVMs:\n%s",
			strings.Join(errList, "\n"),
		)
	}

	return interfaces, nil
}

// getNetworkInterfacesForVM retrieves network interfaces for a single VM
func (client *AzureClient) getNetworkInterfacesForVM(ctx context.Context, vmName string) ([]*armnetwork.Interface, error) {
	vmDetails, err := client.individualvmssVmClient.Get(ctx, client.config.ResourceGroupName, vmName, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get VM details: %w", err)
	}

	var interfaces []*armnetwork.Interface

	if vmDetails.Properties == nil || vmDetails.Properties.NetworkProfile == nil || vmDetails.Properties.NetworkProfile.NetworkInterfaces == nil {
		log.Printf("VM %s has no network interfaces", vmName)
		return interfaces, nil // VM has no network interfaces
	}

	for _, nicRef := range vmDetails.Properties.NetworkProfile.NetworkInterfaces {
		if nicRef.ID == nil {
			continue
		}

		nicName, err := extractResourceNameFromID(*nicRef.ID)
		if err != nil {
			return nil, fmt.Errorf("invalid NIC ID format: %w", err)
		}

		nic, err := client.iFaceClient.Get(ctx, client.config.ResourceGroupName, nicName, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to get network interface %s: %w", nicName, err)
		}

		interfaces = append(interfaces, &nic.Interface)
	}

	return interfaces, nil
}

// GetPrivateIPsForScalingGroup returns the list of IP addresses of instances of the Virtual Machine Scale Set.
func (client *AzureClient) GetPrivateIPsForScalingGroup(name string) ([]string, error) {
	ctx := context.TODO()

	// Validate input
	if name == "" {
		return nil, errors.New("VMSS name cannot be empty")
	}

	// Get scale set details to determine orchestration mode
	vmss, err := client.vMSSClient.Get(ctx, client.config.ResourceGroupName, name, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get scale set %s: %w", name, err)
	}

	if vmss.Properties == nil {
		return nil, fmt.Errorf("scale set %s has no properties", name)
	}

	// Determine orchestration mode
	var orchestrationMode armcompute.OrchestrationMode
	if vmss.Properties.OrchestrationMode != nil {
		orchestrationMode = *vmss.Properties.OrchestrationMode
	}

	// Route to appropriate handler based on orchestration mode
	switch orchestrationMode {
	case armcompute.OrchestrationModeUniform:
		return client.getIPsFromUniformVMSS(ctx, name)
	case armcompute.OrchestrationModeFlexible:
		return client.getIPsFromFlexibleVMSS(ctx, name)
	default:
		return nil, fmt.Errorf("unsupported orchestration mode: %s", orchestrationMode)
	}
}

// getIPsFromUniformVMSS handles uniform orchestration mode using scale set level APIs
func (client *AzureClient) getIPsFromUniformVMSS(ctx context.Context, name string) ([]string, error) {
	interfaces, err := client.listScaleSetsNetworkInterfaces(ctx, client.config.ResourceGroupName, name)
	if err != nil {
		return nil, fmt.Errorf("failed to list network interfaces for uniform VMSS: %w", err)
	}

	return client.extractPrivateIPsFromInterfaces(interfaces)
}

// getIPsFromFlexibleVMSS handles flexible orchestration mode using individual VM APIs
func (client *AzureClient) getIPsFromFlexibleVMSS(ctx context.Context, name string) ([]string, error) {
	vmList, err := client.listVMsInScaleSet(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to list VMs in flexible VMSS: %w", err)
	}

	if len(vmList) == 0 {
		log.Printf("Scale set %s has no VMs", name)
		return []string{}, nil // Empty scale set
	}

	interfaces, err := client.getInterfacesFromIndividualVMs(ctx, vmList)
	if err != nil {
		return nil, fmt.Errorf("failed to get network interfaces from VMs: %w", err)
	}

	return client.extractPrivateIPsFromInterfaces(interfaces)
}

// listVMsInScaleSet lists all VMs in a scale set
func (client *AzureClient) listVMsInScaleSet(ctx context.Context, name string) ([]*armcompute.VirtualMachineScaleSetVM, error) {
	var vmList []*armcompute.VirtualMachineScaleSetVM

	pager := client.vmssVmClient.NewListPager(client.config.ResourceGroupName, name, nil)
	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list VMs: %w", err)
		}
		vmList = append(vmList, resp.Value...)
	}

	return vmList, nil
}

// extractPrivateIPsFromInterfaces extracts private IP addresses from a list of network interfaces
func (client *AzureClient) extractPrivateIPsFromInterfaces(interfaces []*armnetwork.Interface) ([]string, error) {
	if len(interfaces) == 0 {
		return []string{}, nil
	}

	var ips []string
	for _, iface := range interfaces {
		if iface.Properties.VirtualMachine != nil && iface.Properties.VirtualMachine.ID != nil && iface.Properties.IPConfigurations != nil {
			for _, n := range iface.Properties.IPConfigurations {
				ip := getPrimaryIPFromInterfaceIPConfiguration(n)
				if ip != "" {
					ips = append(ips, ip)
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
	if name == "" {
		return false, errors.New("VMSS name cannot be empty")
	}

	ctx := context.TODO()
	expandType := armcompute.ExpandTypesForGetVMScaleSetsUserData
	vmss, err := client.vMSSClient.Get(ctx, client.config.ResourceGroupName, name, &armcompute.VirtualMachineScaleSetsClientGetOptions{Expand: &expandType})
	if err != nil {
		return false, fmt.Errorf("couldn't check if a Virtual Machine Scale Set with name %s exists: %w", name, err)
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
	client.vmssVmClient = computeClientFactory.NewVirtualMachineScaleSetVMsClient()
	client.individualvmssVmClient = computeClientFactory.NewVirtualMachinesClient()

	iclient, err := armnetwork.NewInterfacesClient(client.config.SubscriptionID, cred, nil)
	if err != nil {
		return fmt.Errorf("couldn't create interfaces client: %w", err)
	}
	client.iFaceClient = iclient

	return nil
}

// extractResourceNameFromID extracts the resource name from an Azure resource ID
// Resource ID format: /subscriptions/{subscription}/resourceGroups/{rg}/providers/{provider}/{resourceType}/{resourceName}
func extractResourceNameFromID(resourceID string) (string, error) {
	parts := strings.Split(resourceID, "/")
	if len(parts) == 0 {
		return "", errors.New("invalid resource ID format: empty ID")
	}
	// The resource name is the last part of the ID
	resourceName := parts[len(parts)-1]
	if resourceName == "" {
		return "", errors.New("invalid resource ID format: empty resource name")
	}
	return resourceName, nil
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
