package main

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	yaml "gopkg.in/yaml.v3"
)

// AWSClient allows you to get the list of IP addresses of instances of an Auto Scaling group. It implements the CloudProvider interface.
type AWSClient struct {
	svcEC2         *ec2.Client
	svcAutoscaling *autoscaling.Client
	config         *awsConfig
}

// NewAWSClient creates and configures an AWSClient.
func NewAWSClient(data []byte) (*AWSClient, error) {
	awsClient := &AWSClient{}
	cfg, err := parseAWSConfig(data)
	if err != nil {
		return nil, fmt.Errorf("error validating config: %w", err)
	}
	awsClient.config = cfg

	err = awsClient.configure()
	if err != nil {
		return nil, fmt.Errorf("error configuring AWS Client: %w", err)
	}

	return awsClient, nil
}

// GetUpstreams returns the Upstreams list.
func (client *AWSClient) GetUpstreams() []Upstream {
	upstreams := make([]Upstream, 0, len(client.config.Upstreams))
	for i := range len(client.config.Upstreams) {
		u := Upstream{
			Name:         client.config.Upstreams[i].Name,
			Port:         client.config.Upstreams[i].Port,
			Kind:         client.config.Upstreams[i].Kind,
			ScalingGroup: client.config.Upstreams[i].AutoscalingGroup,
			MaxConns:     &client.config.Upstreams[i].MaxConns,
			MaxFails:     &client.config.Upstreams[i].MaxFails,
			FailTimeout:  getFailTimeoutOrDefault(client.config.Upstreams[i].FailTimeout),
			SlowStart:    getSlowStartOrDefault(client.config.Upstreams[i].SlowStart),
			InService:    client.config.Upstreams[i].InService,
		}
		upstreams = append(upstreams, u)
	}
	return upstreams
}

// configure configures the AWSClient with necessary parameters.
func (client *AWSClient) configure() error {
	httpClient := http.NewBuildableClient().WithTimeout(connTimeoutInSecs * time.Second)

	if client.config.Region == "self" {
		conf, loadErr := config.LoadDefaultConfig(
			context.TODO(),
			config.WithSharedConfigProfile(client.config.Profile),
			config.WithHTTPClient(httpClient),
		)
		if loadErr != nil {
			return fmt.Errorf("unable to load default AWS config: %w", loadErr)
		}

		imdClient := imds.NewFromConfig(conf)

		response, regionErr := imdClient.GetRegion(context.TODO(), &imds.GetRegionInput{})
		if regionErr != nil {
			return fmt.Errorf("unable to retrieve region from ec2metadata: %w", regionErr)
		}
		client.config.Region = response.Region
	}

	cfg, err := config.LoadDefaultConfig(
		context.TODO(),
		config.WithSharedConfigProfile(client.config.Profile),
		config.WithRegion(client.config.Region),
		config.WithHTTPClient(httpClient),
	)
	if err != nil {
		return fmt.Errorf("unable to load default AWS config: %w", err)
	}

	client.svcEC2 = ec2.NewFromConfig(cfg)

	client.svcAutoscaling = autoscaling.NewFromConfig(cfg)

	return nil
}

// parseAWSConfig parses and validates AWSClient config.
func parseAWSConfig(data []byte) (*awsConfig, error) {
	cfg := &awsConfig{}
	err := yaml.Unmarshal(data, cfg)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling AWS config: %w", err)
	}

	err = validateAWSConfig(cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

// CheckIfScalingGroupExists checks if the Auto Scaling group exists.
func (client *AWSClient) CheckIfScalingGroupExists(name string) (bool, error) {
	params := &ec2.DescribeInstancesInput{
		Filters: []types.Filter{
			{
				Name: aws.String("tag:aws:autoscaling:groupName"),
				Values: []string{
					name,
				},
			},
		},
	}

	response, err := client.svcEC2.DescribeInstances(context.Background(), params)
	if err != nil {
		return false, fmt.Errorf("couldn't check if an AutoScaling group exists: %w", err)
	}

	return len(response.Reservations) > 0, nil
}

// GetPrivateIPsForScalingGroup returns the list of IP addresses of instances of the Auto Scaling group.
func (client *AWSClient) GetPrivateIPsForScalingGroup(name string) ([]string, error) {
	var onlyInService bool
	for _, u := range client.GetUpstreams() {
		if u.ScalingGroup == name && u.InService {
			onlyInService = true
			break
		}
	}
	params := &ec2.DescribeInstancesInput{
		Filters: []types.Filter{
			{
				Name: aws.String("tag:aws:autoscaling:groupName"),
				Values: []string{
					name,
				},
			},
		},
	}

	response, err := client.svcEC2.DescribeInstances(context.Background(), params)
	if err != nil {
		return nil, fmt.Errorf("couldn't describe instances: %w", err)
	}

	if len(response.Reservations) == 0 {
		return nil, fmt.Errorf("autoscaling group %v doesn't exist", name)
	}

	var result []string
	insIDtoIP := make(map[string]string)

	for _, res := range response.Reservations {
		for _, ins := range res.Instances {
			if len(ins.NetworkInterfaces) > 0 && ins.NetworkInterfaces[0].PrivateIpAddress != nil {
				if onlyInService {
					insIDtoIP[*ins.InstanceId] = *ins.NetworkInterfaces[0].PrivateIpAddress
				} else {
					result = append(result, *ins.NetworkInterfaces[0].PrivateIpAddress)
				}
			}
		}
	}
	if onlyInService {
		result, err = client.getInstancesInService(insIDtoIP)
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

// getInstancesInService returns the list of instances that have LifecycleState == InService.
func (client *AWSClient) getInstancesInService(insIDtoIP map[string]string) ([]string, error) {
	const maxItems = 50
	var result []string
	keys := reflect.ValueOf(insIDtoIP).MapKeys()
	instanceIDs := make([]string, len(keys))

	for i := range keys {
		instanceIDs[i] = keys[i].String()
	}

	batches := prepareBatches(maxItems, instanceIDs)
	for _, batch := range batches {
		params := &autoscaling.DescribeAutoScalingInstancesInput{
			InstanceIds: batch,
		}
		response, err := client.svcAutoscaling.DescribeAutoScalingInstances(context.Background(), params)
		if err != nil {
			return nil, fmt.Errorf("couldn't describe AutoScaling instances: %w", err)
		}

		for _, ins := range response.AutoScalingInstances {
			if *ins.LifecycleState == "InService" {
				result = append(result, insIDtoIP[*ins.InstanceId])
			}
		}
	}

	return result, nil
}

func prepareBatches(maxItems int, items []string) [][]string {
	totalBatches := (len(items) + maxItems - 1) / maxItems
	batches := make([][]string, 0, totalBatches)

	for i := 0; i < len(items); i += maxItems {
		end := i + maxItems
		if end > len(items) {
			end = len(items)
		}
		batches = append(batches, items[i:end])
	}

	return batches
}

// Configuration for AWS Cloud Provider.
type awsConfig struct {
	Region    string        `yaml:"region"`
	Profile   string        `yaml:"profile"`
	Upstreams []awsUpstream `yaml:"upstreams"`
}

type awsUpstream struct {
	Name             string `yaml:"name"`
	AutoscalingGroup string `yaml:"autoscaling_group"`
	Kind             string `yaml:"kind"`
	FailTimeout      string `yaml:"fail_timeout"`
	SlowStart        string `yaml:"slow_start"`
	Port             int    `yaml:"port"`
	MaxConns         int    `yaml:"max_conns"`
	MaxFails         int    `yaml:"max_fails"`
	InService        bool   `yaml:"in_service"`
}

func validateAWSConfig(cfg *awsConfig) error {
	if cfg.Region == "" {
		return fmt.Errorf(errorMsgFormat, "region")
	}

	if len(cfg.Upstreams) == 0 {
		return errors.New("there are no upstreams found in the config file")
	}

	for _, ups := range cfg.Upstreams {
		if ups.Name == "" {
			return errors.New(upstreamNameErrorMsg)
		}
		if ups.AutoscalingGroup == "" {
			return fmt.Errorf(upstreamErrorMsgFormat, "autoscaling_group", ups.Name)
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
