// SPDX-License-Identifier: Apache-2.0
// Copyright 2024-2026 CloudSlash Contributors

package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	pb "github.com/DrSkyle/cloudslash-plugin-sdk/shared/proto"
)

// RenderDetail returns a ViewModel and available Actions for a resource.
func RenderDetail(ctx context.Context, client *Client, req *pb.RenderDetailRequest) (*pb.RenderDetailResponse, error) {
	switch req.ResourceType {
	case "ec2":
		return renderEC2Detail(ctx, client, req)
	case "s3":
		return renderS3Detail(ctx, client, req)
	case "rds":
		return renderRDSDetail(ctx, client, req)
	case "lambda":
		return renderLambdaDetail(ctx, client, req)
	default:
		return &pb.RenderDetailResponse{
			View: &pb.ViewModel{
				Content: &pb.ViewModel_TextView{
					TextView: &pb.TextView{
						Content: fmt.Sprintf("Unknown resource type: %s", req.ResourceType),
					},
				},
			},
			Actions: []*pb.Action{},
		}, nil
	}
}

// renderEC2Detail returns detail view for an EC2 instance.
func renderEC2Detail(ctx context.Context, client *Client, req *pb.RenderDetailRequest) (*pb.RenderDetailResponse, error) {
	ec2Client := ec2.NewFromConfig(client.Config)

	resp, err := ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{req.ResourceId},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe instance: %w", err)
	}

	if len(resp.Reservations) == 0 || len(resp.Reservations[0].Instances) == 0 {
		return &pb.RenderDetailResponse{
			View: &pb.ViewModel{
				Content: &pb.ViewModel_TextView{
					TextView: &pb.TextView{
						Content: "Instance not found",
					},
				},
			},
			Actions: []*pb.Action{},
		}, nil
	}

	inst := resp.Reservations[0].Instances[0]

	// Build form view with instance details
	fields := []*pb.FormField{
		{Label: "Instance ID", Value: *inst.InstanceId, Type: "text", ReadOnly: true},
		{Label: "State", Value: string(inst.State.Name), Type: "text", ReadOnly: true},
		{Label: "Type", Value: string(inst.InstanceType), Type: "text", ReadOnly: true},
		{Label: "Region", Value: client.Config.Region, Type: "text", ReadOnly: true},
	}

	if inst.LaunchTime != nil {
		fields = append(fields, &pb.FormField{Label: "Launch Time", Value: inst.LaunchTime.Format("2006-01-02 15:04:05"), Type: "text", ReadOnly: true})
	}

	if inst.PrivateIpAddress != nil {
		fields = append(fields, &pb.FormField{Label: "Private IP", Value: *inst.PrivateIpAddress, Type: "text", ReadOnly: true})
	}

	if inst.PublicIpAddress != nil {
		fields = append(fields, &pb.FormField{Label: "Public IP", Value: *inst.PublicIpAddress, Type: "text", ReadOnly: true})
	}

	if inst.VpcId != nil {
		fields = append(fields, &pb.FormField{Label: "VPC ID", Value: *inst.VpcId, Type: "text", ReadOnly: true})
	}

	if inst.SubnetId != nil {
		fields = append(fields, &pb.FormField{Label: "Subnet ID", Value: *inst.SubnetId, Type: "text", ReadOnly: true})
	}

	propsJSON, _ := json.Marshal(inst)
	fields = append(fields, &pb.FormField{Label: "Properties", Value: string(propsJSON), Type: "code", ReadOnly: true})

	view := &pb.ViewModel{
		Content: &pb.ViewModel_FormView{
			FormView: &pb.FormView{
				Fields: fields,
			},
		},
	}

	// Build actions based on instance state
	actions := []*pb.Action{
		{
			Id:          "start",
			Label:       "Start Instance",
			Description: "Start a stopped instance",
			Dangerous:   false,
		},
		{
			Id:          "stop",
			Label:       "Stop Instance",
			Description: "Stop a running instance",
			Dangerous:   false,
		},
		{
			Id:          "reboot",
			Label:       "Reboot Instance",
			Description: "Reboot the instance",
			Dangerous:   false,
		},
		{
			Id:          "terminate",
			Label:       "Terminate Instance",
			Description: "Permanently terminate the instance (cannot be undone)",
			Dangerous:   true,
		},
	}

	return &pb.RenderDetailResponse{
		View:    view,
		Actions: implementedDetailActions(req.ResourceType, actions),
	}, nil
}

// renderS3Detail returns detail view for an S3 bucket.
func renderS3Detail(ctx context.Context, client *Client, req *pb.RenderDetailRequest) (*pb.RenderDetailResponse, error) {
	s3Client := s3.NewFromConfig(client.Config)

	// Get bucket location
	locResp, err := s3Client.GetBucketLocation(ctx, &s3.GetBucketLocationInput{
		Bucket: aws.String(req.ResourceId),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get bucket location: %w", err)
	}

	// Get bucket versioning
	verResp, err := s3Client.GetBucketVersioning(ctx, &s3.GetBucketVersioningInput{
		Bucket: aws.String(req.ResourceId),
	})
	versioningStatus := "Disabled"
	if verResp != nil && verResp.Status != "" {
		versioningStatus = string(verResp.Status)
	}

	// Get bucket encryption
	encResp, err := s3Client.GetBucketEncryption(ctx, &s3.GetBucketEncryptionInput{
		Bucket: aws.String(req.ResourceId),
	})
	encryptionStatus := "Not configured"
	if err == nil && encResp != nil && len(encResp.ServerSideEncryptionConfiguration.Rules) > 0 {
		encryptionStatus = "Enabled"
	}

	region := "us-east-1"
	if locResp.LocationConstraint != "" {
		region = string(locResp.LocationConstraint)
	}

	fields := []*pb.FormField{
		{Label: "Bucket Name", Value: req.ResourceId, Type: "text", ReadOnly: true},
		{Label: "Region", Value: region, Type: "text", ReadOnly: true},
		{Label: "Versioning", Value: versioningStatus, Type: "text", ReadOnly: true},
		{Label: "Encryption", Value: encryptionStatus, Type: "text", ReadOnly: true},
	}

	view := &pb.ViewModel{
		Content: &pb.ViewModel_FormView{
			FormView: &pb.FormView{
				Fields: fields,
			},
		},
	}

	actions := []*pb.Action{
		{
			Id:          "delete",
			Label:       "Delete Bucket",
			Description: "Permanently delete the bucket and all its contents",
			Dangerous:   true,
		},
		{
			Id:          "enable-versioning",
			Label:       "Enable Versioning",
			Description: "Enable versioning for the bucket",
			Dangerous:   false,
		},
		{
			Id:          "disable-versioning",
			Label:       "Disable Versioning",
			Description: "Disable versioning for the bucket",
			Dangerous:   false,
		},
	}

	return &pb.RenderDetailResponse{
		View:    view,
		Actions: implementedDetailActions(req.ResourceType, actions),
	}, nil
}

// renderRDSDetail returns detail view for an RDS instance.
func renderRDSDetail(ctx context.Context, client *Client, req *pb.RenderDetailRequest) (*pb.RenderDetailResponse, error) {
	fields := []*pb.FormField{
		{Label: "DB Instance Identifier", Value: req.ResourceId, Type: "text", ReadOnly: true},
		{Label: "Engine", Value: "postgres", Type: "text", ReadOnly: true},
		{Label: "Status", Value: "available", Type: "text", ReadOnly: true},
	}

	view := &pb.ViewModel{
		Content: &pb.ViewModel_FormView{
			FormView: &pb.FormView{
				Fields: fields,
			},
		},
	}

	actions := []*pb.Action{
		{
			Id:          "stop",
			Label:       "Stop DB Instance",
			Description: "Stop the database instance",
			Dangerous:   false,
		},
		{
			Id:          "reboot",
			Label:       "Reboot DB Instance",
			Description: "Reboot the database instance",
			Dangerous:   false,
		},
		{
			Id:          "delete",
			Label:       "Delete DB Instance",
			Description: "Permanently delete the database instance",
			Dangerous:   true,
		},
	}

	return &pb.RenderDetailResponse{
		View:    view,
		Actions: implementedDetailActions(req.ResourceType, actions),
	}, nil
}

// renderLambdaDetail returns detail view for a Lambda function.
func renderLambdaDetail(ctx context.Context, client *Client, req *pb.RenderDetailRequest) (*pb.RenderDetailResponse, error) {
	fields := []*pb.FormField{
		{Label: "Function Name", Value: req.ResourceId, Type: "text", ReadOnly: true},
		{Label: "Runtime", Value: "go1.x", Type: "text", ReadOnly: true},
		{Label: "Status", Value: "Active", Type: "text", ReadOnly: true},
	}

	view := &pb.ViewModel{
		Content: &pb.ViewModel_FormView{
			FormView: &pb.FormView{
				Fields: fields,
			},
		},
	}

	actions := []*pb.Action{
		{
			Id:          "invoke",
			Label:       "Invoke Function",
			Description: "Invoke the Lambda function",
			Dangerous:   false,
		},
		{
			Id:          "delete",
			Label:       "Delete Function",
			Description: "Permanently delete the Lambda function",
			Dangerous:   true,
		},
	}

	return &pb.RenderDetailResponse{
		View:    view,
		Actions: implementedDetailActions(req.ResourceType, actions),
	}, nil
}

// Detail menus only advertise operations registered by this plugin's remediator.
func implementedDetailActions(resourceType string, actions []*pb.Action) []*pb.Action {
	var out []*pb.Action
	for _, action := range actions {
		if action == nil {
			continue
		}
		switch resourceType {
		case "ec2":
			if validateRemediationAction("ec2", "instance", action.Id) == nil {
				out = append(out, action)
			}
		case "rds":
			if validateRemediationAction("rds", "db-instance", action.Id) == nil {
				out = append(out, action)
			}
		}
	}
	return out
}
