# Configuration for AWS

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
## Table of Contents

- [Setting up Access to AWS API](#setting-up-access-to-aws-api)
- [nginx-asg-sync Configuration](#nginx-asg-sync-configuration)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Setting up Access to AWS API

nginx-asg-sync uses the AWS API to get the list of IP addresses of the instances of an Auto Scaling group. To access the
AWS API, nginx-asg-sync must have credentials. To provide credentials to nginx-asg-sync:

1. [Create an IAM role](http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/iam-roles-for-amazon-ec2.html) and attach the
   predefined `AmazonEC2ReadOnlyAccess` policy to it. This policy allows read-only access to EC2 APIs.
2. When you launch the NGINX Plus instance, add this IAM role to the instance.

## nginx-asg-sync Configuration

nginx-asg-sync is configured in **/etc/nginx/config.yaml**.

```yaml
region: us-west-2
api_endpoint: http://127.0.0.1:8080/api
sync_interval: 5s
cloud_provider: AWS
profile: default
# Optional: custom headers for NGINX+ requests, for authentication or other requirements
# custom_headers:
#   Content-Type: application/json
upstreams:
  - name: backend-one
    autoscaling_group: backend-one-group
    port: 80
    kind: http
    max_conns: 0
    max_fails: 1
    fail_timeout: 10s
    slow_start: 0s
  - name: backend-two
    autoscaling_group: backend-two-group
    port: 80
    kind: http
    max_conns: 0
    max_fails: 1
    fail_timeout: 10s
    slow_start: 0s
    in_service: true
```

- The `api_endpoint` key defines the NGINX Plus API endpoint.
- The `sync_interval` key defines the synchronization interval: nginx-asg-sync checks for scaling updates
  every 5 seconds. The value is a string that represents a duration (e.g., `5s`). The maximum unit is hours.
- The `cloud_provider` key defines a cloud provider that will be used. The default is `AWS`. This means the key can be
  empty if using AWS. Possible values are: `AWS`, `Azure`.
- The `custom_headers` key (optional) defines custom HTTP headers to be sent with NGINX+ API requests.
- The `region` key defines the AWS region where we deploy NGINX Plus and the Auto Scaling groups. Setting `region` to
  `self` will use the EC2 Metadata service to retrieve the region of the current instance.
- The optional `profile` key specifies the AWS profile to use.
- The `upstreams` key defines the list of upstream groups. For each upstream group we specify:
  - `name` – The name we specified for the upstream block in the NGINX Plus configuration.
  - `autoscaling_group` – The name of the corresponding Auto Scaling group. Use of wildcards is supported. For example,
    `backend-*`.
  - `port` – The port on which our backend applications are exposed.
  - `kind` – The protocol of the traffic NGINX Plus load balances to the backend application, here `http`. If the
    application uses TCP/UDP, specify `stream` instead.
  - `max_conns` – The maximum number of simultaneous active connections to an upstream server. Default value is 0,
    meaning there is no limit.
  - `max_fails` – The number of unsuccessful attempts to communicate with an upstream server that should happen in the
    duration set by the `fail-timeout` to consider the server unavailable. Default value is 1. The zero value disables
    the accounting of attempts.
  - `fail_timeout` – The time during which the specified number of unsuccessful attempts to communicate with an upstream
    server should happen to consider the server unavailable. Default value is 10s.
  - `slow_start` – The slow start allows an upstream server to gradually recover its weight from 0 to its nominal value
    after it has been recovered or became available or when the server becomes available after a period of time it was
    considered unavailable. By default, the slow start is disabled.
  - `in_service` – Use only instances that are in the `InService` state of the
    [Lifecycle](https://docs.aws.amazon.com/autoscaling/ec2/userguide/AutoScalingGroupLifecycle.html). Default value is
    false.
