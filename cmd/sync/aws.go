package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	yaml "gopkg.in/yaml.v2"
)

// AWSClient allows you to get the list of IP addresses of instances of an Auto Scaling group. It implements the CloudProvider interface
type AWSClient struct {
	svcEC2         *ec2.Client
	svcAutoscaling *autoscaling.Client
	config         *awsConfig
}

// NewAWSClient creates and configures an AWSClient
func NewAWSClient(data []byte) (*AWSClient, error) {
	awsClient := &AWSClient{}
	cfg, err := parseAWSConfig(data)
	if err != nil {
		return nil, fmt.Errorf("error validating config: %w", err)
	}

	if cfg.Region == "self" {
		httpClient := &http.Client{Timeout: connTimeoutInSecs * time.Second}

		conf, loadErr := config.LoadDefaultConfig(context.TODO())
		if loadErr != nil {
			return nil, loadErr
		}

		client := imds.NewFromConfig(conf, func(o *imds.Options) {
			o.HTTPClient = httpClient
		})

		response, regionErr := client.GetRegion(context.TODO(), &imds.GetRegionInput{})
		if regionErr != nil {
			return nil, fmt.Errorf("unable to retrieve region from ec2metadata: %w", regionErr)
		}
		cfg.Region = response.Region
	}

	awsClient.config = cfg

	err = awsClient.configure()
	if err != nil {
		return nil, fmt.Errorf("error configuring AWS Client: %w", err)
	}

	return awsClient, nil
}

// GetUpstreams returns the Upstreams list
func (client *AWSClient) GetUpstreams() []Upstream {
	var upstreams []Upstream
	for i := 0; i < len(client.config.Upstreams); i++ {
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

// configure configures the AWSClient with necessary parameters
func (client *AWSClient) configure() error {
	httpClient := &http.Client{Timeout: connTimeoutInSecs * time.Second}

	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return err
	}

	client.svcEC2 = ec2.NewFromConfig(cfg, func(o *ec2.Options) {
		o.Region = client.config.Region
		o.HTTPClient = httpClient
	})

	client.svcAutoscaling = autoscaling.NewFromConfig(cfg, func(o *autoscaling.Options) {
		o.Region = client.config.Region
		o.HTTPClient = httpClient
	})

	return nil
}

// parseAWSConfig parses and validates AWSClient config
func parseAWSConfig(data []byte) (*awsConfig, error) {
	cfg := &awsConfig{}
	err := yaml.Unmarshal(data, cfg)
	if err != nil {
		return nil, err
	}

	err = validateAWSConfig(cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

// CheckIfScalingGroupExists checks if the Auto Scaling group exists
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

// GetPrivateIPsForScalingGroup returns the list of IP addresses of instances of the Auto Scaling group
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
		return nil, err
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

// getInstancesInService returns the list of instances that have LifecycleState == InService
func (client *AWSClient) getInstancesInService(insIDtoIP map[string]string) ([]string, error) {
	const maxItems = 50
	var result []string
	keys := reflect.ValueOf(insIDtoIP).MapKeys()
	instanceIDs := make([]string, len(keys))

	for i := 0; i < len(keys); i++ {
		instanceIDs[i] = keys[i].String()
	}

	batches := prepareBatches(maxItems, instanceIDs)
	for _, batch := range batches {
		params := &autoscaling.DescribeAutoScalingInstancesInput{
			InstanceIds: batch,
		}
		response, err := client.svcAutoscaling.DescribeAutoScalingInstances(context.Background(), params)
		if err != nil {
			return nil, err
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
	var batches [][]string

	min := func(a, b int) int {
		if a <= b {
			return a
		}
		return b
	}

	for i := 0; i < len(items); i += maxItems {
		batches = append(batches, items[i:min(i+maxItems, len(items))])
	}
	return batches
}

// Configuration for AWS Cloud Provider

type awsConfig struct {
	Region    string
	Upstreams []awsUpstream
}

type awsUpstream struct {
	Name             string
	AutoscalingGroup string `yaml:"autoscaling_group"`
	Kind             string
	FailTimeout      string `yaml:"fail_timeout"`
	SlowStart        string `yaml:"slow_start"`
	Port             int
	MaxConns         int  `yaml:"max_conns"`
	MaxFails         int  `yaml:"max_fails"`
	InService        bool `yaml:"in_service"`
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
		if ups.Kind == "" || !(ups.Kind == "http" || ups.Kind == "stream") {
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
