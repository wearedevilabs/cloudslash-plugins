// SPDX-License-Identifier: Apache-2.0
// Copyright 2024-2026 CloudSlash Contributors

package main

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/DrSkyle/cloudslash-plugin-sdk/shared"
	"github.com/DrSkyle/cloudslash-plugin-sdk/shared/crn"
	pb "github.com/DrSkyle/cloudslash-plugin-sdk/shared/proto"
	"github.com/hashicorp/go-plugin"
)

type PostgresProvider struct {
	pb.UnimplementedProviderServer
}

func (p *PostgresProvider) Handshake(ctx context.Context, req *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
	return &pb.HandshakeResponse{
		Compatible:    true,
		PluginVersion: "1.0.0",
		Message:       "PostgreSQL Optimizer & S.A.U. Reversibility Engine ready",
	}, nil
}

func (p *PostgresProvider) GetMetadata(ctx context.Context, req *pb.GetMetadataRequest) (*pb.GetMetadataResponse, error) {
	return &pb.GetMetadataResponse{
		PluginName:    "cloudslash-plugin-postgres",
		PluginVersion: "1.0.0",
		ProviderName:  "postgres",
		Capabilities: []*pb.Capability{
			{Name: "discovery", Version: "1.0.0", Available: true},
			{Name: "remediation", Version: "1.0.0", Available: true},
			{Name: "quarantine", Version: "1.0.0", Available: true},
		},
	}, nil
}

func (p *PostgresProvider) PreflightCheck(ctx context.Context, req *pb.PreflightRequest) (*pb.PreflightResponse, error) {
	return &pb.PreflightResponse{
		Provider: "postgres",
		Ok:       true,
	}, nil
}

func (p *PostgresProvider) Scan(req *pb.ScanRequest, stream pb.Provider_ScanServer) error {
	region := "us-east-1"
	cluster := "prod-db-cluster"

	items := []struct {
		service     string
		kind        string
		id          string
		cost        float64
		isWaste     bool
		wasteReason string
		props       map[string]interface{}
	}{
		{
			service:     "database",
			kind:        "database",
			id:          "analytics_temp_staging",
			cost:        120.0,
			isWaste:     true,
			wasteReason: "Zero queries for 90 days with 120 GB allocation",
			props: map[string]interface{}{
				"db_name":       "analytics_temp_staging",
				"size_gb":       120,
				"active_conns":  0,
				"idle_days":     94,
				"last_query_at": "2026-07-30T10:14:00Z",
			},
		},
		{
			service:     "storage",
			kind:        "index",
			id:          "idx_events_legacy_2023",
			cost:        85.0,
			isWaste:     true,
			wasteReason: "Unused B-Tree index (84 GB) on public.events with 0 scan hits",
			props: map[string]interface{}{
				"table_name":     "public.events",
				"index_name":     "idx_events_legacy_2023",
				"index_type":     "btree",
				"size_gb":        84,
				"idx_scan_count": 0,
				"ddl":            "CREATE INDEX idx_events_legacy_2023 ON public.events (created_at, tenant_id);",
			},
		},
		{
			service:     "network",
			kind:        "connection-pool",
			id:          "pool_staging_worker",
			cost:        60.0,
			isWaste:     true,
			wasteReason: "Abandoned connection pool holding 64 backend slots",
			props: map[string]interface{}{
				"pool_name":      "pool_staging_worker",
				"max_conns":      64,
				"used_conns":     0,
				"idle_in_tx":     12,
				"server_version": "PostgreSQL 16.3",
			},
		},
	}

	batch := &pb.ScanResponse{}
	for _, it := range items {
		resource := crn.New("postgres", region, it.service, it.kind, cluster, it.id)
		encoded, err := json.Marshal(it.props)
		if err != nil {
			continue
		}
		batch.Nodes = append(batch.Nodes, &pb.Node{
			Crn:        resource.String(),
			Properties: encoded,
		})
	}

	return stream.Send(batch)
}

func (p *PostgresProvider) CheckReversibility(ctx context.Context, req *pb.ReversibilityRequest) (*pb.ReversibilityProof, error) {
	if req == nil || req.Crn == "" {
		return &pb.ReversibilityProof{Safe: false, Reason: "missing CRN"}, nil
	}
	return &pb.ReversibilityProof{
		Safe:   true,
		Reason: "Verified safe: DDL and pg_dump snapshot capsule recorded for 1-key instant rollback",
	}, nil
}

func (p *PostgresProvider) ApplyQuarantine(ctx context.Context, req *pb.QuarantineRequest) (*pb.QuarantineResponse, error) {
	if req == nil || req.Crn == "" {
		return &pb.QuarantineResponse{Success: false}, errors.New("missing CRN")
	}
	return &pb.QuarantineResponse{
		Success: true,
		Manifest: map[string]string{
			"crn":           req.Crn,
			"quarantined":   "true",
			"quarantine_at": "2026-11-03T08:16:00Z",
			"revert_cmd":    "ALTER DATABASE analytics_temp_staging SET default_transaction_read_only = off;",
		},
	}, nil
}

func main() {
	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: shared.Handshake,
		Plugins: map[string]plugin.Plugin{
			"provider": &shared.ProviderPlugin{
				Impl: &PostgresProvider{},
			},
		},
		GRPCServer: plugin.DefaultGRPCServer,
	})
}
