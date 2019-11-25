package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	nginx "github.com/nginxinc/nginx-plus-go-client/client"
)

var configFile = flag.String("config_path", "/etc/nginx/config.yaml", "Path to the config file")
var logFile = flag.String("log_path", "", "Path to the log file. If the file doesn't exist, it will be created")
var version = "0.4-1"

const connTimeoutInSecs = 10

func main() {
	flag.Parse()

	if *logFile != "" {
		logF, err := os.OpenFile(*logFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
		if err != nil {
			log.Printf("Couldn't open the log file: %v", err)
			os.Exit(10)
		}
		log.SetOutput(io.MultiWriter(logF, os.Stderr))
	}

	log.Printf("nginx-asg-sync version %s", version)

	cfgData, err := ioutil.ReadFile(*configFile)
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

	httpClient := &http.Client{Timeout: connTimeoutInSecs * time.Second}
	nginxClient, err := nginx.NewNginxClient(httpClient, commonConfig.APIEndpoint)

	if err != nil {
		log.Printf("Couldn't create NGINX client: %v", err)
		os.Exit(10)
	}

	upstreams := cloudProviderClient.GetUpstreams()
	if err != nil {
		log.Printf("Couldn't get Upstreams: %v", err)
		os.Exit(10)
	}

	for _, ups := range upstreams {
		if ups.Kind == "http" {
			err = nginxClient.CheckIfUpstreamExists(ups.Name)
		} else {
			err = nginxClient.CheckIfStreamUpstreamExists(ups.Name)
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

				added, removed, updated, err := nginxClient.UpdateHTTPServers(upstream.Name, upsServers)
				if err != nil {
					log.Printf("Couldn't update HTTP servers in NGINX: %v", err)
					continue
				}

				if len(added) > 0 || len(removed) > 0 || len(updated) > 0 {
					log.Printf("Updated HTTP servers of %v; Added: %+v, Removed: %+v, Updated: %+v", upstream, added, removed, updated)
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

				added, removed, updated, err := nginxClient.UpdateStreamServers(upstream.Name, upsServers)
				if err != nil {
					log.Printf("Couldn't update Steam servers in NGINX: %v", err)
					continue
				}

				if len(added) > 0 || len(removed) > 0 || len(updated) > 0 {
					log.Printf("Updated Stream servers of %v; Added: %+v, Removed: %+v, Updated: %+v", upstream, added, removed, updated)
				}
			}

		}

		select {
		case <-time.After(commonConfig.SyncIntervalInSeconds * time.Second):
		case <-sigterm:
			log.Println("Terminating...")
			return
		}
	}
}
