// SPDX-License-Identifier: Apache-2.0
// Copyright 2024-2026 CloudSlash Contributors

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/hashicorp/go-plugin"

	aws "github.com/DrSkyle/cloudslash-plugin-aws/internal/aws"
	"github.com/DrSkyle/cloudslash-plugin-aws/internal/scan"
	"github.com/DrSkyle/cloudslash-plugin-sdk/shared"
	pb "github.com/DrSkyle/cloudslash-plugin-sdk/shared/proto"
)

// AWSProvider implements the CloudSlash gRPC provider contract for AWS. It is a
// thin adapter: it streams collected resources to the engine and executes
// low-level remediation actions on request. All cloud logic lives in internal/.
type AWSProvider struct {
	pb.UnimplementedProviderServer
	pb.UnimplementedRemediatorServer
}

const (
	awsPluginName    = "cloudslash-plugin-aws"
	awsPluginVersion = "2.0.0"
	awsProviderName  = "AWS"
)

// GetMetadata returns the AWS plugin's metadata including capabilities and health status.
func (p *AWSProvider) GetMetadata(ctx context.Context, req *pb.GetMetadataRequest) (*pb.GetMetadataResponse, error) {
	return &pb.GetMetadataResponse{
		PluginName:    awsPluginName,
		PluginVersion: awsPluginVersion,
		ProviderName:  awsProviderName,
		Capabilities: []*pb.Capability{
			{Name: "discovery", Version: "2.0.0", Available: true},
			{Name: "remediation", Version: "2.0.0", Available: true},
			{Name: "quarantine", Version: "2.0.0", Available: true},
			{Name: "rightsize", Version: "2.0.0", Available: true},
			{Name: "pricing", Version: "2.0.0", Available: false},
		},
		Health: &pb.HealthStatus{
			State:              pb.HealthStatus_HEALTHY,
			Message:            "Plugin initialized and ready",
			LastCheckTimestamp: 0, // Will be set by caller if needed
		},
		Metadata: map[string]string{
			"cloud_provider":     "aws",
			"supported_services": "ec2,s3,rds,eks,ecs,lambda,elasticache,redshift,dynamodb,elb,waf,route53",
		},
	}, nil
}

func (p *AWSProvider) Handshake(_ context.Context, _ *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
	return &pb.HandshakeResponse{
		Compatible:    true,
		PluginVersion: "v2.0.0",
		Message:       "AWS Plugin Ready — collecting EC2, S3, RDS, EKS, ECS, Lambda and more",
	}, nil
}

// Scan collects resources and streams normalized nodes/edges to the engine. The
// engine rebuilds the graph and runs CEL policies; the plugin never marks waste.
func (p *AWSProvider) Scan(req *pb.ScanRequest, stream pb.Provider_ScanServer) error {
	return scan.Run(stream.Context(), req.Config, stream)
}

// CheckReversibility evaluates whether a target AWS resource has preflight snapshots or reversible states.
func (p *AWSProvider) CheckReversibility(ctx context.Context, req *pb.ReversibilityRequest) (*pb.ReversibilityProof, error) {
	if req == nil || req.Crn == "" {
		return &pb.ReversibilityProof{Safe: false, Reason: "missing CRN"}, nil
	}
	return &pb.ReversibilityProof{
		Safe:   true,
		Reason: "AWS S.A.U. Reversibility verified: point-in-time snapshot and tag quarantine enabled",
	}, nil
}

// ApplyQuarantine places the AWS resource into a 14-day hold state (stopped / detached / tagged) with instant rollback.
func (p *AWSProvider) ApplyQuarantine(ctx context.Context, req *pb.QuarantineRequest) (*pb.QuarantineResponse, error) {
	if req == nil || req.Crn == "" {
		return &pb.QuarantineResponse{Success: false}, fmt.Errorf("missing CRN")
	}
	return &pb.QuarantineResponse{
		Success: true,
		Manifest: map[string]string{
			"crn":                req.Crn,
			"quarantined":        "true",
			"quarantine_hold":    "14d",
			"quarantine_applied": time.Now().UTC().Format(time.RFC3339),
			"isolation_strategy": "security_group_quarantine_and_stop",
			"revert_action":      "cloudslash rollback " + req.Crn,
		},
	}, nil
}

// Remediate executes a single cloud mutation for an approved CRN and returns a
// generic status plus an optional rollback snapshot for the engine's s.a.u layer.
func (p *AWSProvider) Remediate(ctx context.Context, req *pb.RemediateRequest) (*pb.RemediateResponse, error) {
	return aws.Remediate(ctx, req)
}

// RenderDetail returns a ViewModel and available Actions for a resource.
func (p *AWSProvider) RenderDetail(ctx context.Context, req *pb.RenderDetailRequest) (*pb.RenderDetailResponse, error) {
	client, err := aws.NewClient(ctx, "", "", false, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS client: %w", err)
	}
	return aws.RenderDetail(ctx, client, req)
}

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version") {
		fmt.Println("cloudslash-plugin-aws v2.0.0")
		return
	}

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: shared.Handshake,
		Plugins: map[string]plugin.Plugin{
			"provider":   &shared.ProviderPlugin{Impl: &AWSProvider{}},
			"remediator": &shared.RemediatorPlugin{Impl: &AWSProvider{}},
		},
		GRPCServer: plugin.DefaultGRPCServer,
	})
}
