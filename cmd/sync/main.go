package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	nginx "github.com/nginx/nginx-plus-go-client/v2/client"
)

var (
	configFile = flag.String("config_path", "/etc/nginx/config.yaml", "Path to the config file")
	logFile    = flag.String("log_path", "", "Path to the log file. If the file doesn't exist, it will be created")
	version    string
)

const (
	connTimeoutInSecs = 10
	maxHeaders        = 100
)

func main() {
	flag.Parse()

	if *logFile != "" {
		logF, err := os.OpenFile(*logFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
		if err != nil {
			log.Printf("Couldn't open the log file: %v", err)
			os.Exit(10)
		}
		log.SetOutput(io.MultiWriter(logF, os.Stderr))
	}

	log.Printf("nginx-asg-sync version %s", version)

	cfgData, err := os.ReadFile(*configFile)
	if err != nil {
		log.Printf("Couldn't read the config file %v: %v", *configFile, err)
		os.Exit(10)
	}

	commonConfig, err := parseCommonConfig(cfgData)
	if err != nil {
		log.Printf("Couldn't parse the config: %v", err)
		os.Exit(10)
	}

	var cloudProviderClient CloudProvider

	switch commonConfig.CloudProvider {
	case "AWS":
		cloudProviderClient, err = NewAWSClient(cfgData)
	case "Azure":
		cloudProviderClient, err = NewAzureClient(cfgData)
	}

	if err != nil {
		log.Printf("Couldn't create cloud provider client for %v: %v", commonConfig.CloudProvider, err)
		os.Exit(10)
	}

	httpClient := NewHTTPClient(commonConfig)
	nginxClient, err := nginx.NewNginxClient(commonConfig.APIEndpoint, nginx.WithHTTPClient(httpClient))
	if err != nil {
		log.Printf("Couldn't create NGINX client: %v", err)
		os.Exit(10)
	}

	upstreams := cloudProviderClient.GetUpstreams()

	for _, ups := range upstreams {
		ctx := context.TODO()
		if ups.Kind == "http" {
			err = nginxClient.CheckIfUpstreamExists(ctx, ups.Name)
		} else {
			err = nginxClient.CheckIfStreamUpstreamExists(ctx, ups.Name)
		}

		if err != nil {
			log.Printf("Problem with the NGINX configuration: %v", err)
			os.Exit(10)
		}

		exists, err := cloudProviderClient.CheckIfScalingGroupExists(ups.ScalingGroup)
		if err != nil {
			log.Printf("Couldn't check if Scaling group exists: %v", err)
			os.Exit(10)
		} else if !exists {
			log.Printf("Warning: Scaling group '%v' doesn't exist in the cloud provider", ups.ScalingGroup)
		}
	}

	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGTERM)

	for {
		for _, upstream := range upstreams {
			ips, err := cloudProviderClient.GetPrivateIPsForScalingGroup(upstream.ScalingGroup)
			if err != nil {
				log.Printf("Couldn't get the IP addresses for %v: %v", upstream.ScalingGroup, err)
				continue
			}
			ctx := context.TODO()

			if upstream.Kind == "http" {
				var upsServers []nginx.UpstreamServer
				for _, ip := range ips {
					backend := fmt.Sprintf("%v:%v", ip, upstream.Port)
					upsServers = append(upsServers, nginx.UpstreamServer{
						Server:      backend,
						MaxConns:    upstream.MaxConns,
						MaxFails:    upstream.MaxFails,
						FailTimeout: upstream.FailTimeout,
						SlowStart:   upstream.SlowStart,
					})
				}

				added, removed, updated, err := nginxClient.UpdateHTTPServers(ctx, upstream.Name, upsServers)
				if err != nil {
					log.Printf("Couldn't update HTTP servers in NGINX: %v", err)
					continue
				}

				if len(added) > 0 || len(removed) > 0 || len(updated) > 0 {
					addedAddresses := getUpstreamServerAddresses(added)
					removedAddresses := getUpstreamServerAddresses(removed)
					updatedAddresses := getUpstreamServerAddresses(updated)
					log.Printf("Updated HTTP servers of %v for group %v ; Added: %+v, Removed: %+v, Updated: %+v",
						upstream.Name, upstream.ScalingGroup, addedAddresses, removedAddresses, updatedAddresses)
				}
			} else {
				var upsServers []nginx.StreamUpstreamServer
				for _, ip := range ips {
					backend := fmt.Sprintf("%v:%v", ip, upstream.Port)
					upsServers = append(upsServers, nginx.StreamUpstreamServer{
						Server:      backend,
						MaxConns:    upstream.MaxConns,
						MaxFails:    upstream.MaxFails,
						FailTimeout: upstream.FailTimeout,
						SlowStart:   upstream.SlowStart,
					})
				}

				added, removed, updated, err := nginxClient.UpdateStreamServers(ctx, upstream.Name, upsServers)
				if err != nil {
					log.Printf("Couldn't update Steam servers in NGINX: %v", err)
					continue
				}

				if len(added) > 0 || len(removed) > 0 || len(updated) > 0 {
					addedAddresses := getStreamUpstreamServerAddresses(added)
					removedAddresses := getStreamUpstreamServerAddresses(removed)
					updatedAddresses := getStreamUpstreamServerAddresses(updated)
					log.Printf("Updated Stream servers of %v for group %v ; Added: %+v, Removed: %+v, Updated: %+v",
						upstream.Name, upstream.ScalingGroup, addedAddresses, removedAddresses, updatedAddresses)
				}
			}
		}

		select {
		case <-time.After(commonConfig.SyncInterval):
		case <-sigterm:
			log.Println("Terminating...")
			return
		}
	}
}

func getUpstreamServerAddresses(server []nginx.UpstreamServer) []string {
	upstreamServerAddr := make([]string, 0, len(server))
	for _, s := range server {
		upstreamServerAddr = append(upstreamServerAddr, s.Server)
	}
	return upstreamServerAddr
}

func getStreamUpstreamServerAddresses(server []nginx.StreamUpstreamServer) []string {
	streamUpstreamServerAddr := make([]string, 0, len(server))
	for _, s := range server {
		streamUpstreamServerAddr = append(streamUpstreamServerAddr, s.Server)
	}
	return streamUpstreamServerAddr
}

// headerTransport wraps an http.RoundTripper and adds custom headers to all requests.
type headerTransport struct {
	headers   http.Header
	transport http.RoundTripper
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if len(t.headers)+len(req.Header) > maxHeaders {
		return nil, fmt.Errorf("number of headers in request exceeds the maximum allowed (%d)", maxHeaders)
	}
	clonedReq := req.Clone(req.Context())

	for key, values := range t.headers {
		for _, value := range values {
			clonedReq.Header.Add(key, value)
		}
	}

	resp, err := t.transport.RoundTrip(clonedReq)
	if err != nil {
		return nil, fmt.Errorf("headerTransport RoundTrip failed: %w", err)
	}

	return resp, nil
}

func NewHTTPClient(cfg *commonConfig) *http.Client {
	var rt http.RoundTripper = &http.Transport{}
	if h := NewHeaders(cfg); len(h) > 0 {
		rt = &headerTransport{headers: h, transport: rt}
	}
	return &http.Client{
		Transport: rt,
		Timeout:   connTimeoutInSecs * time.Second,
	}
}

func NewHeaders(cfg *commonConfig) http.Header {
	headers := http.Header{}

	for key, value := range cfg.CustomHeaders {
		headers.Set(key, value)
	}
	return headers
}
