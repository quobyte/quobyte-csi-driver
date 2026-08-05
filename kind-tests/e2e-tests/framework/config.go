package framework

import (
	"os"
	"testing"
)

// Config holds the environment-provided settings needed to run the e2e suite
// against an already-deployed kind cluster + CSI driver + Quobyte client.
type Config struct {
	Kubeconfig         string
	Namespace          string
	QuobyteAPIURL      string
	QuobyteAPIUser     string
	QuobyteAPIPassword string
	QuobyteTenant      string
	CSIProvisionerName string
}

// LoadConfig reads the required environment variables and fails the test
// immediately if any of them are missing.
func LoadConfig(t *testing.T) Config {
	t.Helper()

	cfg := Config{
		Kubeconfig:         os.Getenv("KUBECONFIG"),
		Namespace:          os.Getenv("NAMESPACE"),
		QuobyteAPIURL:      os.Getenv("QUOBYTE_API_URL"),
		QuobyteAPIUser:     os.Getenv("QUOBYTE_API_USER"),
		QuobyteAPIPassword: os.Getenv("QUOBYTE_API_PASSWORD"),
		QuobyteTenant:      os.Getenv("QUOBYTE_TENANT"),
		CSIProvisionerName: os.Getenv("CSI_PROVISIONER_NAME"),
	}

	required := map[string]string{
		"KUBECONFIG":           cfg.Kubeconfig,
		"NAMESPACE":            cfg.Namespace,
		"QUOBYTE_API_URL":      cfg.QuobyteAPIURL,
		"QUOBYTE_API_USER":     cfg.QuobyteAPIUser,
		"QUOBYTE_API_PASSWORD": cfg.QuobyteAPIPassword,
		"QUOBYTE_TENANT":       cfg.QuobyteTenant,
		"CSI_PROVISIONER_NAME": cfg.CSIProvisionerName,
	}
	for name, value := range required {
		if value == "" {
			t.Fatalf("required environment variable %s is not set", name)
		}
	}

	return cfg
}
