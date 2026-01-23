package main

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v7"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v8"
)

type testInputAzure struct {
	cfg *azureConfig
	msg string
}

type mockVMSSClient struct {
	getFunc func(ctx context.Context, rg, name string, opts *armcompute.VirtualMachineScaleSetsClientGetOptions) (armcompute.VirtualMachineScaleSetsClientGetResponse, error)
}

func (m *mockVMSSClient) Get(ctx context.Context, rg, name string, opts *armcompute.VirtualMachineScaleSetsClientGetOptions) (armcompute.VirtualMachineScaleSetsClientGetResponse, error) {
	return m.getFunc(ctx, rg, name, opts)
}

type mockVMSSVMsClient struct {
	newListPagerFunc func(rg, name string, opts *armcompute.VirtualMachineScaleSetVMsClientListOptions) *mockPagerVMSSVMs
}

func (m *mockVMSSVMsClient) NewListPager(
	rg, name string, opts *armcompute.VirtualMachineScaleSetVMsClientListOptions,
) *runtime.Pager[armcompute.VirtualMachineScaleSetVMsClientListResponse] {
	pager := m.newListPagerFunc(rg, name, opts)
	return runtime.NewPager(
		runtime.PagingHandler[armcompute.VirtualMachineScaleSetVMsClientListResponse]{
			More: func(
				_ armcompute.VirtualMachineScaleSetVMsClientListResponse,
			) bool {
				return pager.More()
			},
			Fetcher: func(
				ctx context.Context,
				_ *armcompute.VirtualMachineScaleSetVMsClientListResponse,
			) (
				armcompute.VirtualMachineScaleSetVMsClientListResponse,
				error,
			) {
				return pager.NextPage(ctx)
			},
		},
	)
}

type mockVMsClient struct {
	getFunc func(ctx context.Context, rg, name string, opts *armcompute.VirtualMachinesClientGetOptions) (armcompute.VirtualMachinesClientGetResponse, error)
}

func (m *mockVMsClient) Get(ctx context.Context, rg, name string, opts *armcompute.VirtualMachinesClientGetOptions) (armcompute.VirtualMachinesClientGetResponse, error) {
	return m.getFunc(ctx, rg, name, opts)
}

type mockInterfacesClient struct {
	newListPagerFunc func(rg, vmss string, opts *armnetwork.InterfacesClientListVirtualMachineScaleSetNetworkInterfacesOptions) *mockPagerNICs
	getFunc          func(ctx context.Context, rg, name string, opts *armnetwork.InterfacesClientGetOptions) (armnetwork.InterfacesClientGetResponse, error)
}

func (m *mockInterfacesClient) NewListVirtualMachineScaleSetNetworkInterfacesPager(
	rg, vmss string,
	opts *armnetwork.InterfacesClientListVirtualMachineScaleSetNetworkInterfacesOptions,
) *runtime.Pager[armnetwork.InterfacesClientListVirtualMachineScaleSetNetworkInterfacesResponse] {
	pager := m.newListPagerFunc(rg, vmss, opts)
	return runtime.NewPager(
		runtime.PagingHandler[armnetwork.InterfacesClientListVirtualMachineScaleSetNetworkInterfacesResponse]{
			More: func(_ armnetwork.InterfacesClientListVirtualMachineScaleSetNetworkInterfacesResponse) bool {
				return pager.More()
			},
			Fetcher: func(ctx context.Context, _ *armnetwork.InterfacesClientListVirtualMachineScaleSetNetworkInterfacesResponse) (armnetwork.InterfacesClientListVirtualMachineScaleSetNetworkInterfacesResponse, error) {
				return pager.NextPage(ctx)
			},
		},
	)
}

func (m *mockInterfacesClient) Get(ctx context.Context, rg, name string, opts *armnetwork.InterfacesClientGetOptions) (armnetwork.InterfacesClientGetResponse, error) {
	return m.getFunc(ctx, rg, name, opts)
}

type mockPagerNICs struct {
	err   error
	pages [][]*armnetwork.Interface
	idx   int
}

func (m *mockPagerNICs) More() bool {
	return m.idx < len(m.pages)
}

func (m *mockPagerNICs) NextPage(_ context.Context) (armnetwork.InterfacesClientListVirtualMachineScaleSetNetworkInterfacesResponse, error) {
	if m.err != nil {
		return armnetwork.InterfacesClientListVirtualMachineScaleSetNetworkInterfacesResponse{}, m.err
	}
	if m.idx >= len(m.pages) {
		return armnetwork.InterfacesClientListVirtualMachineScaleSetNetworkInterfacesResponse{}, errors.New("no more pages")
	}
	resp := armnetwork.InterfacesClientListVirtualMachineScaleSetNetworkInterfacesResponse{
		InterfaceListResult: armnetwork.InterfaceListResult{
			Value: m.pages[m.idx],
		},
	}
	m.idx++
	return resp, nil
}

type mockPagerVMSSVMs struct {
	err   error
	pages [][]*armcompute.VirtualMachineScaleSetVM
	idx   int
}

func (m *mockPagerVMSSVMs) More() bool {
	return m.idx < len(m.pages)
}

func (m *mockPagerVMSSVMs) NextPage(_ context.Context) (armcompute.VirtualMachineScaleSetVMsClientListResponse, error) {
	if m.err != nil {
		return armcompute.VirtualMachineScaleSetVMsClientListResponse{}, m.err
	}
	if m.idx >= len(m.pages) {
		return armcompute.VirtualMachineScaleSetVMsClientListResponse{}, errors.New("no more pages")
	}
	resp := armcompute.VirtualMachineScaleSetVMsClientListResponse{
		VirtualMachineScaleSetVMListResult: armcompute.VirtualMachineScaleSetVMListResult{
			Value: m.pages[m.idx],
		},
	}
	m.idx++
	return resp, nil
}

func TestAzureClient_GetPrivateIPsForScalingGroup(t *testing.T) {
	t.Parallel()
	uniformVMSS := armcompute.VirtualMachineScaleSetsClientGetResponse{
		VirtualMachineScaleSet: armcompute.VirtualMachineScaleSet{
			Properties: &armcompute.VirtualMachineScaleSetProperties{
				OrchestrationMode: func() *armcompute.OrchestrationMode {
					m := armcompute.OrchestrationModeUniform
					return &m
				}(),
			},
		},
	}
	flexibleVMSS := armcompute.VirtualMachineScaleSetsClientGetResponse{
		VirtualMachineScaleSet: armcompute.VirtualMachineScaleSet{
			Properties: &armcompute.VirtualMachineScaleSetProperties{
				OrchestrationMode: func() *armcompute.OrchestrationMode {
					m := armcompute.OrchestrationModeFlexible
					return &m
				}(),
			},
		},
	}
	//nolint:govet
	tests := []struct {
		vmssResp        armcompute.VirtualMachineScaleSetsClientGetResponse
		uniformNICs     [][]*armnetwork.Interface
		flexibleVMs     [][]*armcompute.VirtualMachineScaleSetVM
		flexibleNICs    map[string][]*armnetwork.Interface
		flexibleNICsErr map[string]error
		wantIPs         []string
		vmssErr         error
		uniformNICsErr  error
		flexibleVMsErr  error
		wantErr         bool
		orchestration   armcompute.OrchestrationMode
		name            string
	}{
		{
			name:     "Uniform - single NIC with primary IP",
			vmssResp: uniformVMSS,
			uniformNICs: [][]*armnetwork.Interface{{{
				Properties: &armnetwork.InterfacePropertiesFormat{
					VirtualMachine: &armnetwork.SubResource{
						ID: ptrStr("/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Compute/virtualMachines/vm1"),
					},
					IPConfigurations: []*armnetwork.InterfaceIPConfiguration{{
						Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
							Primary:          ptrBool(true),
							PrivateIPAddress: ptrStr("10.0.0.1"),
						},
					}},
				},
			}}},
			wantIPs: []string{"10.0.0.1"},
		},
		{
			name:        "Uniform - no NICs",
			vmssResp:    uniformVMSS,
			uniformNICs: [][]*armnetwork.Interface{{}},
			wantIPs:     []string{},
		},
		{
			name:           "Uniform - error listing NICs",
			vmssResp:       uniformVMSS,
			uniformNICsErr: errors.New("fail list NICs"),
			wantErr:        true,
		},
		{
			name:     "Flexible - single VM, single NIC",
			vmssResp: flexibleVMSS,
			flexibleVMs: [][]*armcompute.VirtualMachineScaleSetVM{{{
				Name: ptrStr("vm1"),
			}}},
			flexibleNICs: map[string][]*armnetwork.Interface{
				"vm1-nic": {{
					Properties: &armnetwork.InterfacePropertiesFormat{
						VirtualMachine: &armnetwork.SubResource{
							ID: ptrStr("/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Compute/virtualMachines/vm1"),
						},
						IPConfigurations: []*armnetwork.InterfaceIPConfiguration{{
							Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
								Primary:          ptrBool(true),
								PrivateIPAddress: ptrStr("10.0.0.2"),
							},
						}},
					},
				}},
			},
			wantIPs: []string{"10.0.0.2"},
		},
		{
			name:           "Flexible - error listing VMs",
			vmssResp:       flexibleVMSS,
			flexibleVMsErr: errors.New("fail list VMs"),
			wantErr:        true,
		},
		{
			name:     "Flexible - error getting NICs",
			vmssResp: flexibleVMSS,
			flexibleVMs: [][]*armcompute.VirtualMachineScaleSetVM{{{
				Name: ptrStr("vm2"),
			}}},
			flexibleNICsErr: map[string]error{
				"vm2-nic": errors.New("fail get NICs"),
			},
			wantErr: true,
		},
		{
			name: "unknown orchestration mode",
			vmssResp: armcompute.VirtualMachineScaleSetsClientGetResponse{
				VirtualMachineScaleSet: armcompute.VirtualMachineScaleSet{
					Properties: &armcompute.VirtualMachineScaleSetProperties{
						OrchestrationMode: func() *armcompute.OrchestrationMode {
							m := armcompute.OrchestrationMode("UnknownMode")
							return &m
						}(),
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ac := &AzureClient{
				config: &azureConfig{
					SubscriptionID:    "sub",
					ResourceGroupName: "rg",
				},
			}

			ac.vMSSClient = &mockVMSSClient{
				getFunc: func(_ context.Context, _, _ string, _ *armcompute.VirtualMachineScaleSetsClientGetOptions) (armcompute.VirtualMachineScaleSetsClientGetResponse, error) {
					return tt.vmssResp, tt.vmssErr
				},
			}

			ac.vmssVMClient = &mockVMSSVMsClient{
				newListPagerFunc: func(_, _ string, _ *armcompute.VirtualMachineScaleSetVMsClientListOptions) *mockPagerVMSSVMs {
					return &mockPagerVMSSVMs{
						pages: tt.flexibleVMs,
						err:   tt.flexibleVMsErr,
					}
				},
			}

			ac.individualvmssVMClient = &mockVMsClient{
				getFunc: func(_ context.Context, _, name string, _ *armcompute.VirtualMachinesClientGetOptions) (armcompute.VirtualMachinesClientGetResponse, error) {
					// Only used in flexible mode
					if tt.flexibleNICsErr != nil && tt.flexibleNICsErr[name] != nil {
						return armcompute.VirtualMachinesClientGetResponse{}, tt.flexibleNICsErr[name]
					}
					return armcompute.VirtualMachinesClientGetResponse{
						VirtualMachine: armcompute.VirtualMachine{
							Properties: &armcompute.VirtualMachineProperties{
								NetworkProfile: &armcompute.NetworkProfile{
									NetworkInterfaces: []*armcompute.NetworkInterfaceReference{{
										ID: ptrStr("/subscriptions/sub/resourceGroups/rg/providers/Microsoft.armnetwork/networkInterfaces/" + name + "-nic"),
									}},
								},
							},
						},
					}, nil
				},
			}

			ac.iFaceClient = &mockInterfacesClient{
				newListPagerFunc: func(_, _ string, _ *armnetwork.InterfacesClientListVirtualMachineScaleSetNetworkInterfacesOptions) *mockPagerNICs {
					return &mockPagerNICs{
						pages: tt.uniformNICs,
						err:   tt.uniformNICsErr,
					}
				},
				getFunc: func(_ context.Context, _, name string, _ *armnetwork.InterfacesClientGetOptions) (armnetwork.InterfacesClientGetResponse, error) {
					// Only used in flexible mode
					if tt.flexibleNICsErr != nil && tt.flexibleNICsErr[name] != nil {
						return armnetwork.InterfacesClientGetResponse{}, tt.flexibleNICsErr[name]
					}
					if tt.flexibleNICs != nil && tt.flexibleNICs[name] != nil {
						return armnetwork.InterfacesClientGetResponse{
							Interface: armnetwork.Interface{
								Properties: tt.flexibleNICs[name][0].Properties,
							},
						}, nil
					}
					return armnetwork.InterfacesClientGetResponse{}, nil
				},
			}

			ips, err := ac.GetPrivateIPsForScalingGroup("testvmss")
			if (err != nil) != tt.wantErr {
				t.Fatalf("expected error: %v, got: %v", tt.wantErr, err)
			}
			if !reflect.DeepEqual(ips, tt.wantIPs) {
				t.Errorf("expected IPs: %v, got: %v", tt.wantIPs, ips)
			}
		})
	}
}

func ptrStr(s string) *string { return &s }
func ptrBool(b bool) *bool    { return &b }

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
	input := make([]*testInputAzure, 0, 10)

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
	ipConfig := &armnetwork.InterfaceIPConfiguration{
		Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
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
		ipConfig *armnetwork.InterfaceIPConfiguration
		msg      string
	}{
		{
			ipConfig: &armnetwork.InterfaceIPConfiguration{},
			msg:      "empty primary",
		},
		{
			ipConfig: &armnetwork.InterfaceIPConfiguration{
				Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
					Primary: &primaryFalse,
				},
			},
			msg: "not primary interface",
		},
		{
			ipConfig: &armnetwork.InterfaceIPConfiguration{
				Properties: nil,
			},
			msg: "no interface properties",
		},
		{
			ipConfig: &armnetwork.InterfaceIPConfiguration{
				Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
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
