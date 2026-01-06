# NGINXaaS for Azure Configuration

This document provides configuration guidance specifically for [NGINXaaS for Azure](https://docs.nginx.com/nginxaas/azure).

## Requirements

1. **API Endpoint**: Use the [dataplane API endpoint](https://docs.nginx.com/nginxaas/azure/loadbalancer-kubernetes/#nginxaas-data-plane-api-endpoint)
    from your NGINXaaS deployment with `/nplus` suffix
2. **Authentication Headers**:
   - `Content-Type: application/json`
   - `Authorization: ApiKey <base64_encoded_dataplane_key>`
3. **Dataplane API Key**: Obtain the [dataplane api key](https://docs.nginx.com/nginxaas/azure/loadbalancer-kubernetes#create-an-nginxaas-data-plane-api-key-using-the-azure-portal)
from Azure portal and base64 encode it

## Configuration Example

Below is a complete configuration example for nginx-asg-sync with NGINXaaS for Azure:

```yaml
# Example configuration for NGINXaaS for Azure
# This configuration is specifically for NGINXaaS Service on Azure

cloud_provider: Azure
subscription_id: your_subscription_id
resource_group_name: your_resource_group

# NGINXaaS for Azure dataplane API endpoint
# Replace with your actual NGINXaaS dataplane API endpoint from Azure portal
# See the docs: https://docs.nginx.com/nginxaas/azure/loadbalancer-kubernetes/#nginxaas-data-plane-api-endpoint
api_endpoint: ${dataplaneApiEndpoint}/nplus
# Sample end_point : https://your-instance-name.region.nginxaas.nginxlab.net/nplus
sync_interval: 5s

# Required headers for NGINXaaS for Azure
custom_headers:
  Content-Type: application/json
  # Authorization header with your dataplane API key (base64 encoded)
  # Get this key from your NGINXaaS deployment in the Azure portal and base64 encode it.
  # See the docs : https://docs.nginx.com/nginxaas/azure/loadbalancer-kubernetes/#create-an-nginxaas-data-plane-api-key-using-the-azure-portal
  Authorization: ApiKey your_base64_encoded_dataplane_api_key

upstreams:
  - name: backend-one
    virtual_machine_scale_set: backend-one-vmss
    port: 80
    kind: http
    max_conns: 0
    max_fails: 1
    fail_timeout: 10s
    slow_start: 0s
  - name: backend-two
    virtual_machine_scale_set: backend-two-vmss
    port: 80
    kind: http
    max_conns: 0
    max_fails: 1
    fail_timeout: 10s
    slow_start: 0s
  - name: tcp-backend
    virtual_machine_scale_set: backend-three-vmss
    port: 3306
    kind: stream
    max_conns: 0
    max_fails: 1
    fail_timeout: 10s
    slow_start: 0s
```
