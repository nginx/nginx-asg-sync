package main

import (
	"testing"
)

type testInputAWS struct {
	cfg *awsConfig
	msg string
}

func getValidAWSConfig() *awsConfig {
	upstreams := []awsUpstream{
		{
			Name:             "backend1",
			AutoscalingGroup: "backend-group",
			Port:             80,
			Kind:             "http",
			InService:        false,
		},
	}
	cfg := awsConfig{
		Region:    "us-west-2",
		Upstreams: upstreams,
		Profile:   "default",
	}

	return &cfg
}

func getInvalidAWSConfigInput() []*testInputAWS {
	input := make([]*testInputAWS, 0, 10)

	invalidRegionCfg := getValidAWSConfig()
	invalidRegionCfg.Region = ""
	input = append(input, &testInputAWS{invalidRegionCfg, "invalid region"})

	invalidMissingUpstreamsCfg := getValidAWSConfig()
	invalidMissingUpstreamsCfg.Upstreams = nil
	input = append(input, &testInputAWS{invalidMissingUpstreamsCfg, "no upstreams"})

	invalidUpstreamNameCfg := getValidAWSConfig()
	invalidUpstreamNameCfg.Upstreams[0].Name = ""
	input = append(input, &testInputAWS{invalidUpstreamNameCfg, "invalid name of the upstream"})

	invalidUpstreamAutoscalingGroupCfg := getValidAWSConfig()
	invalidUpstreamAutoscalingGroupCfg.Upstreams[0].AutoscalingGroup = ""
	input = append(input, &testInputAWS{invalidUpstreamAutoscalingGroupCfg, "invalid autoscaling_group of the upstream"})

	invalidUpstreamPortCfg := getValidAWSConfig()
	invalidUpstreamPortCfg.Upstreams[0].Port = 0
	input = append(input, &testInputAWS{invalidUpstreamPortCfg, "invalid port of the upstream"})

	invalidUpstreamKindCfg := getValidAWSConfig()
	invalidUpstreamKindCfg.Upstreams[0].Kind = ""
	input = append(input, &testInputAWS{invalidUpstreamKindCfg, "invalid kind of the upstream"})

	invalidUpstreamMaxConnsCfg := getValidAWSConfig()
	invalidUpstreamMaxConnsCfg.Upstreams[0].MaxConns = -10
	input = append(input, &testInputAWS{invalidUpstreamMaxConnsCfg, "invalid max_conns of the upstream"})

	invalidUpstreamMaxFailsCfg := getValidAWSConfig()
	invalidUpstreamMaxFailsCfg.Upstreams[0].MaxFails = -10
	input = append(input, &testInputAWS{invalidUpstreamMaxFailsCfg, "invalid max_fails of the upstream"})

	invalidUpstreamFailTimeoutCfg := getValidAWSConfig()
	invalidUpstreamFailTimeoutCfg.Upstreams[0].FailTimeout = "-10s"
	input = append(input, &testInputAWS{invalidUpstreamFailTimeoutCfg, "invalid fail_timeout of the upstream"})

	invalidUpstreamSlowStartCfg := getValidAWSConfig()
	invalidUpstreamSlowStartCfg.Upstreams[0].SlowStart = "-10s"
	input = append(input, &testInputAWS{invalidUpstreamSlowStartCfg, "invalid slow_start of the upstream"})

	return input
}

func TestValidateAWSConfigNotValid(t *testing.T) {
	t.Parallel()
	input := getInvalidAWSConfigInput()

	for _, item := range input {
		err := validateAWSConfig(item.cfg)
		if err == nil {
			t.Errorf("validateAWSConfig() didn't fail for the invalid config file with %v", item.msg)
		}
	}
}

func TestValidateAWSConfigValid(t *testing.T) {
	t.Parallel()
	cfg := getValidAWSConfig()

	err := validateAWSConfig(cfg)
	if err != nil {
		t.Errorf("validateAWSConfig() failed for the valid config: %v", err)
	}
}

func TestGetUpstreamsAWS(t *testing.T) {
	t.Parallel()
	cfg := getValidAWSConfig()
	upstreams := []awsUpstream{
		{
			Name:        "127.0.0.1",
			Port:        80,
			MaxFails:    1,
			MaxConns:    2,
			SlowStart:   "5s",
			FailTimeout: "10s",
			InService:   false,
		},
		{
			Name:        "127.0.0.2",
			Port:        80,
			MaxFails:    2,
			MaxConns:    3,
			SlowStart:   "6s",
			FailTimeout: "11s",
			InService:   true,
		},
	}
	cfg.Upstreams = upstreams
	c := AWSClient{config: cfg}

	ups := c.GetUpstreams()
	for _, u := range ups {
		found := false
		for _, cfgU := range cfg.Upstreams {
			if u.Name == cfgU.Name {
				if !areEqualUpstreamsAWS(cfgU, u) {
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

func areEqualUpstreamsAWS(u1 awsUpstream, u2 Upstream) bool {
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

	if u1.InService != u2.InService {
		return false
	}

	return true
}

func TestPrepareBatches(t *testing.T) {
	t.Parallel()
	const maxItems = 3
	ids := []string{"i-394ujfs", "i-dfdinf", "i-fsfsf", "i-8hr83hfwif", "i-nsnsnan"}
	instanceIDs := make([]string, len(ids))

	copy(instanceIDs, ids)

	batches := prepareBatches(maxItems, instanceIDs)

	if len(batches) > len(ids)/maxItems+1 {
		t.Error("prepareBatches() didn't split the slice correctly")
	}

	for _, batch := range batches {
		if len(batch) > maxItems {
			t.Errorf("prepareBatches() returned a batch with len > %v", maxItems)
		}
	}
}
