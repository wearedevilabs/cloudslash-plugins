// SPDX-License-Identifier: Apache-2.0
// Copyright 2024-2026 CloudSlash Contributors

package aws

import (
	"context"
	"testing"

	pb "github.com/DrSkyle/cloudslash-plugin-sdk/shared/proto"
)

// TestRenderDetailUnknownType tests that an unknown resource type returns an appropriate response.
func TestRenderDetailUnknownType(t *testing.T) {
	client := &Client{}
	req := &pb.RenderDetailRequest{
		ResourceId:   "test-resource",
		ResourceType: "unknown-type",
		PaneContext:  "detail",
	}

	resp, err := RenderDetail(context.Background(), client, req)
	if err != nil {
		t.Fatalf("RenderDetail returned error: %v", err)
	}

	if resp.View == nil {
		t.Fatal("View should not be nil")
	}

	textView, ok := resp.View.Content.(*pb.ViewModel_TextView)
	if !ok {
		t.Fatal("Expected TextView for unknown type")
	}

	if textView.TextView == nil || textView.TextView.Content == "" {
		t.Fatal("TextView content should not be empty")
	}

	if len(resp.Actions) != 0 {
		t.Errorf("Expected no actions for unknown type, got %d", len(resp.Actions))
	}
}

// Menu coverage is independent of live credentials and never calls AWS.
func TestDetailActionsExcludeUnimplementedOperations(t *testing.T) {
	actions := []*pb.Action{{Id: "start"}, {Id: "stop"}, {Id: "terminate"}, {Id: "delete"}, {Id: "reboot"}, {Id: "invoke"}, nil}
	for _, test := range []struct {
		resource string
		want     int
	}{{"ec2", 4}, {"rds", 3}, {"s3", 0}, {"lambda", 0}, {"unknown", 0}} {
		t.Run(test.resource, func(t *testing.T) {
			got := implementedDetailActions(test.resource, actions)
			if len(got) != test.want {
				t.Fatalf("action count %d, want %d", len(got), test.want)
			}
			for _, action := range got {
				if action.Id == "reboot" || action.Id == "invoke" {
					t.Fatalf("unimplemented action %s", action.Id)
				}
			}
		})
	}
}
