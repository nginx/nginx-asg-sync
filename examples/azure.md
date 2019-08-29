# Configuration for Azure

## Setting up Access to Azure API

nginx-asg-sync uses the Azure API to get the list of IP addresses of the instances of a Virtual Machine Scale Set. To access the Azure API, nginx-asg-sync must have credentials. To provide credentials to nginx-asg-sync:

1. Create the NGINX Plus VM with the system or user [identity](https://docs.microsoft.com/en-us/azure/active-directory/managed-identities-azure-resources/qs-configure-portal-windows-vm#system-assigned-managed-identity).
2. Create a `Reader` [role](https://docs.microsoft.com/en-us/azure/role-based-access-control/overview) for the selected subscription or resource group and [assign it to the](https://docs.microsoft.com/en-gb/azure/role-based-access-control/role-assignments-portal#add-a-role-assignment) identity of the NGINX Plus VM.

## nginx-asg-sync Configuration

nginx-asg-sync is configured in **/etc/nginx/config.yaml**.

```yaml
api_endpoint: http://127.0.0.1:8080/api
sync_interval_in_seconds: 5
cloud_provider: Azure
subscription_id: my_subscription_id
resource_group: my_resource_group
upstreams:
 - name: backend-one
   virtual_machine_scale_set: backend-one-group
   port: 80
   kind: http
 - name: backend-two
   virtual_machine_scale_set: backend-two-group
   port: 80
   kind: http
```

* The `api_endpoint` key defines the NGINX Plus API endpoint.
* The `sync_interval_in_seconds` key defines the synchronization interval: nginx-asg-sync checks for scaling updates every 5 seconds.
* The `cloud_provider` key defines a Cloud Provider that will be used. The default is `AWS`. This means the key can be empty if using AWS. Possible values are: `AWS`, `Azure`.
* The `subscription_id` key defines the Azure unique subscription id that identifies your Azure subscription.
* The `resource_group` key defines the Azure resource group of your Virtual Machine Scale Set and Virtual Machine for NGINX Plus.
* The `upstreams` key defines the list of upstream groups. For each upstream group we specify:
  * `name` – The name we specified for the upstream block in the NGINX Plus configuration.
  * `virtual_machine_scale_set` – The name of the corresponding Virtual Machine Scale Set.
  * `port` – The port on which our backend applications are exposed.
  * `kind` – The protocol of the traffic NGINX Plus load balances to the backend application, here `http`. If the application uses TCP/UDP, specify `stream` instead.