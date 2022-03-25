package config

// Config contains configuration values for yale to run
type Config struct {
	SecretData []struct {
		SecretName    string `yaml:"secretName"`
		Namespace     string `yaml:"namespace"`
		SecretDataKey string `yaml:"secretDataKey"`
		GcpSaName     string `yaml:"gcpSaName"`
	} `yaml:"secretData"`
	GoogleProject string `yaml:"googleProject"`
}
