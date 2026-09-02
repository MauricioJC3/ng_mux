package main

import (
	"strings"
	"testing"
)

func TestRefuseIfNested(t *testing.T) {
	t.Run("not nested", func(t *testing.T) {
		t.Setenv(nestGuardEnv, "")
		if err := refuseIfNested(); err != nil {
			t.Fatalf("want nil, got %v", err)
		}
	})

	t.Run("nested", func(t *testing.T) {
		t.Setenv(nestGuardEnv, "default")
		err := refuseIfNested()
		if err == nil {
			t.Fatal("want an error when NGMUX is set")
		}
		if !strings.Contains(err.Error(), "unset "+nestGuardEnv) {
			t.Fatalf("error should tell the user how to force: %v", err)
		}
	})
}
