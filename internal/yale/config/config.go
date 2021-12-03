package config

import (
	"fmt"
	yaml "gopkg.in/yaml.v3"
	"io/ioutil"
)

// Config contains configuration values for yale to run
type Config struct {
	SecretData [] struct {
		SecretName string    `yaml:"secretName"`
		Namespace string `yaml:"namespace""`
		SecretDataKey string `yaml:"secretDataKey"`
		GcpSaName   string `yaml:"gcpSaName"`
	} `yaml:"secretData"`
	GoogleProject    string `yaml:"googleProject"`
}

// Read attempts to parse the file at configPath and create build a config struct from it
func Read(configPath string) (*Config, error) {
	configBytes, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("Error reading config file: %v", err)
	}
	config := new(Config)
	if err := yaml.Unmarshal(configBytes, config); err != nil {
		return nil, fmt.Errorf("Error parsing config: %v", err)
	}
	return config, nil
}