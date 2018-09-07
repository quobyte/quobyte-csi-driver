package driver

import "testing"

func TestMount(t *testing.T) {
	err := Mount("/mnt/quobyte", "/mnt/quobyte_1", "", []string{})
	if err != nil {
		t.Errorf("error: %v", err)
	}
}
