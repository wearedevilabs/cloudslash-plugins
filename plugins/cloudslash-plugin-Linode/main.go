// SPDX-License-Identifier: Apache-2.0
// Copyright 2024-2026 CloudSlash Contributors

package main

import (
	context "context"
	"errors"
	"os"
	"strings"

	"github.com/DrSkyle/cloudslash-plugin-sdk/shared"
	pb "github.com/DrSkyle/cloudslash-plugin-sdk/shared/proto"
	"github.com/hashicorp/go-plugin"
)

type Provider struct {
	pb.UnimplementedProviderServer
}

func (p *Provider) Handshake(ctx context.Context, req *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
	return &pb.HandshakeResponse{Compatible: true, PluginVersion: "1.0.0"}, nil
}

func (p *Provider) Scan(req *pb.ScanRequest, stream pb.Provider_ScanServer) error {
	if req == nil {
		return errors.New("scan configuration is required")
	}
	account := strings.TrimSpace(req.Config["account_id"])
	if account == "" {
		account = strings.TrimSpace(os.Getenv("LINODE_ACCOUNT_ID"))
	}
	token := strings.TrimSpace(os.Getenv("LINODE_TOKEN"))
	return scanResources(stream.Context(), newLinodeClient(token), account, stream.Send)
}

func (p *Provider) ApplyQuarantine(ctx context.Context, req *pb.QuarantineRequest) (*pb.QuarantineResponse, error) {
	return &pb.QuarantineResponse{Success: false}, errors.New("Linode plugin supports read-only discovery; quarantine is not implemented")
}

func (p *Provider) CheckReversibility(ctx context.Context, req *pb.ReversibilityRequest) (*pb.ReversibilityProof, error) {
	return &pb.ReversibilityProof{Safe: false, Reason: "read-only discovery does not establish action reversibility"}, nil
}

func (p *Provider) GetMetadata(context.Context, *pb.GetMetadataRequest) (*pb.GetMetadataResponse, error) {
	return &pb.GetMetadataResponse{PluginName: "cloudslash-plugin-Linode", PluginVersion: "1.0.0", ProviderName: "linode",
		Capabilities: []*pb.Capability{{Name: "discovery", Version: "1.0.0", Available: true}}}, nil
}

func (p *Provider) PreflightCheck(ctx context.Context, _ *pb.PreflightRequest) (*pb.PreflightResponse, error) {
	client := newLinodeClient(strings.TrimSpace(os.Getenv("LINODE_TOKEN")))
	for _, family := range linodeFamilies {
		if _, err := client.page(ctx, family.path, 1); err != nil {
			return &pb.PreflightResponse{Provider: "linode", Error: err.Error()}, nil
		}
	}
	return &pb.PreflightResponse{Provider: "linode", Ok: true}, nil
}

func main() {
	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: shared.Handshake,
		Plugins: map[string]plugin.Plugin{
			"provider": &shared.ProviderPlugin{
				Impl: &Provider{},
			},
		},
		GRPCServer: plugin.DefaultGRPCServer,
	})
}
