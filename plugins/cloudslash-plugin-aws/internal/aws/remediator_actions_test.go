package aws

import (
	"context"
	"testing"

	sdkaws "github.com/aws/aws-sdk-go-v2/aws"
)

func TestUnsupportedActionsNeverReachProvider(t *testing.T) {
	for _, action := range []string{"", "resize", "typo", "STOP"} {
		response, err := remediateEC2Instance(context.Background(), sdkaws.Config{}, "i-test", action, false)
		if err != nil || response.Success || response.Error == "" {
			t.Fatalf("instance accepted %q", action)
		}
		response, err = remediateEBSVolume(context.Background(), sdkaws.Config{}, "vol-test", action, false)
		if err != nil || response.Success || response.Error == "" {
			t.Fatalf("volume accepted %q", action)
		}
		response, err = remediateRDS(context.Background(), sdkaws.Config{}, "db-test", action, false)
		if err != nil || response.Success || response.Error == "" {
			t.Fatalf("database accepted %q", action)
		}
	}
	for _, action := range []string{"start", "stop"} {
		response, err := remediateEBSVolume(context.Background(), sdkaws.Config{}, "vol-test", action, true)
		if err != nil || response.Success {
			t.Fatalf("volume preflight accepted %q", action)
		}
	}
	if err := validateRemediationAction("ec2", "instance", "delete"); err != nil {
		t.Fatal(err)
	}
}
