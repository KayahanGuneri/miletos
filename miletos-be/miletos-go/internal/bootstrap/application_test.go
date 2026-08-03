package bootstrap

import "testing"

func TestValidateServerPort(t *testing.T) {
	for _, port := range []int{1, 65535} {
		if err := validateServerPort("PORT", port); err != nil {
			t.Fatalf("validateServerPort(%d) error = %v", port, err)
		}
	}
	for _, port := range []int{-1, 0, 65536} {
		if err := validateServerPort("PORT", port); err == nil {
			t.Fatalf("validateServerPort(%d) error = nil", port)
		}
	}
}
