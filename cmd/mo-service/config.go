// Copyright 2022 Matrix Origin
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/matrixorigin/matrixone/pkg/cnservice"
	"github.com/matrixorigin/matrixone/pkg/common/moerr"
	"github.com/matrixorigin/matrixone/pkg/config"
	"github.com/matrixorigin/matrixone/pkg/dnservice"
	"github.com/matrixorigin/matrixone/pkg/fileservice"
	"github.com/matrixorigin/matrixone/pkg/logservice"
	"github.com/matrixorigin/matrixone/pkg/logutil"
)

const (
	cnServiceType         = "CN"
	dnServiceType         = "DN"
	logServiceType        = "LOG"
	standaloneServiceType = "STANDALONE"

	s3FileServiceName    = "S3"
	localFileServiceName = "LOCAL"
	etlFileServiceName   = "ETL"
)

var (
	supportServiceTypes = map[string]any{
		cnServiceType:         cnServiceType,
		dnServiceType:         dnServiceType,
		logServiceType:        logServiceType,
		standaloneServiceType: standaloneServiceType,
	}
)

// Config mo-service configuration
type Config struct {
	// Log log config
	Log logutil.LogConfig `toml:"log"`
	// ServiceType service type, select the corresponding configuration to start the
	// service according to the service type. [CN|DN|Log|Standalone]
	ServiceType string `toml:"service-type"`
	// FileServices the config for file services
	FileServices []fileservice.Config `toml:"fileservice"`
	// HAKeeperClient hakeeper client config
	HAKeeperClient logservice.HAKeeperClientConfig `toml:"hakeeper-client"`
	// DN dn service config
	DN dnservice.Config `toml:"dn"`
	// LogService is the config for log service
	LogService logservice.Config `toml:"logservice"`
	// CN cn service config
	CN cnservice.Config `toml:"cn"`
	// Observability parameters for the metric/trace
	Observability config.ObservabilityParameters `toml:"observability"`
}

func parseConfigFromFile(file string) (*Config, error) {
	if file == "" {
		return nil, fmt.Errorf("toml config file not set")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return parseFromString(string(data))
}

func parseFromString(data string) (*Config, error) {
	cfg := &Config{}
	if _, err := toml.Decode(data, cfg); err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if err := cfg.resolveGossipSeedAddresses(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	if _, ok := supportServiceTypes[strings.ToUpper(c.ServiceType)]; !ok {
		return fmt.Errorf("service type %s not support", c.ServiceType)
	}
	return nil
}

func (c *Config) createFileService(defaultName string) (*fileservice.FileServices, error) {
	// create all services
	services := make([]fileservice.FileService, 0, len(c.FileServices))
	for _, config := range c.FileServices {
		service, err := fileservice.NewFileService(config)
		if err != nil {
			return nil, err
		}
		services = append(services, service)
	}

	// create FileServices
	fs, err := fileservice.NewFileServices(
		defaultName,
		services...,
	)
	if err != nil {
		return nil, err
	}

	// validate default name
	_, err = fileservice.Get[fileservice.FileService](fs, defaultName)
	if err != nil {
		return nil, err
	}

	// ensure local exists
	_, err = fileservice.Get[fileservice.FileService](fs, localFileServiceName)
	if err != nil {
		return nil, err
	}

	// ensure s3 exists
	_, err = fileservice.Get[fileservice.FileService](fs, s3FileServiceName)
	if err != nil {
		return nil, err
	}

	// ensure etl exists, for trace & metric
	if !c.Observability.DisableMetric || !c.Observability.DisableTrace {
		_, err = fileservice.Get[fileservice.FileService](fs, etlFileServiceName)
		if err != nil {
			return nil, moerr.ConvertPanicError(err)
		}
	}

	return fs, nil
}

func (c *Config) getLogServiceConfig() logservice.Config {
	cfg := c.LogService
	logutil.Infof("hakeeper client cfg: %v", c.HAKeeperClient)
	cfg.HAKeeperClientConfig = c.HAKeeperClient
	return cfg
}

func (c *Config) getDNServiceConfig() dnservice.Config {
	cfg := c.DN
	cfg.HAKeeper.ClientConfig = c.HAKeeperClient
	return cfg
}

func (c *Config) getCNServiceConfig() cnservice.Config {
	cfg := c.CN
	cfg.HAKeeper.ClientConfig = c.HAKeeperClient
	cfg.Frontend.SetLogAndVersion(&c.Log, Version)
	return cfg
}

func (c *Config) getObservabilityConfig() config.ObservabilityParameters {
	cfg := c.Observability
	cfg.SetDefaultValues(Version)
	return cfg
}

// memberlist requires all gossip seed addresses to be provided as IP:PORT
func (c *Config) resolveGossipSeedAddresses() error {
	result := make([]string, 0)
	for _, addr := range c.LogService.GossipSeedAddresses {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return err
		}
		ips, err := net.LookupIP(host)
		if err != nil {
			// the configured member may be failed currently, keep the host name anyway since
			// memberlist would try to resolve it again
			result = append(result, addr)
			continue
		}
		// only keep IPv4 addresses
		filtered := make([]string, 0)
		for _, ip := range ips {
			if ip.To4() != nil {
				filtered = append(filtered, ip.String())
			}
		}
		if len(filtered) != 1 {
			return moerr.NewBadConfig("GossipSeedAddress %s", addr)
		}
		result = append(result, net.JoinHostPort(filtered[0], port))
	}
	c.LogService.GossipSeedAddresses = result
	return nil
}
