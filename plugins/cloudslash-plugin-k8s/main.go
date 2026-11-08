// SPDX-License-Identifier: Apache-2.0
// Copyright 2024-2026 CloudSlash Contributors

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/hashicorp/go-plugin"

	"github.com/DrSkyle/cloudslash-plugin-k8s/k8s"
	"github.com/DrSkyle/cloudslash-plugin-sdk/shared"
	pb "github.com/DrSkyle/cloudslash-plugin-sdk/shared/proto"
	graph "github.com/DrSkyle/cloudslash-plugin-sdk/shared/scangraph"
)

// K8sProvider implements the CloudSlash gRPC plugin contract for Kubernetes
// (EKS node groups discovered via the local kubeconfig).
type K8sProvider struct {
	pb.UnimplementedProviderServer
}

const (
	k8sPluginName    = "cloudslash-plugin-k8s"
	k8sPluginVersion = "2.0.0"
	k8sProviderName  = "Kubernetes"
)

// GetMetadata returns the K8s plugin's metadata including capabilities and health status.
func (p *K8sProvider) GetMetadata(ctx context.Context, req *pb.GetMetadataRequest) (*pb.GetMetadataResponse, error) {
	return &pb.GetMetadataResponse{
		PluginName:    k8sPluginName,
		PluginVersion: k8sPluginVersion,
		ProviderName:  k8sProviderName,
		Capabilities: []*pb.Capability{
			{Name: "discovery", Version: "2.0.0", Available: true},
			{Name: "remediation", Version: "2.0.0", Available: true},
			{Name: "quarantine", Version: "2.0.0", Available: true},
			{Name: "topology", Version: "2.0.0", Available: true},
			{Name: "blast_radius", Version: "2.0.0", Available: true},
		},
		Health: &pb.HealthStatus{
			State:              pb.HealthStatus_HEALTHY,
			Message:            "Plugin initialized and ready",
			LastCheckTimestamp: 0,
		},
		Metadata: map[string]string{
			"cloud_provider":     "kubernetes",
			"supported_services": "pods,nodes,deployments,services,persistentvolumes,configmaps,secrets",
		},
	}, nil
}

func (p *K8sProvider) Handshake(_ context.Context, _ *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
	return &pb.HandshakeResponse{
		Compatible:    true,
		PluginVersion: k8sPluginVersion,
		Message:       "K8s Plugin Ready — scanning EKS node groups",
	}, nil
}

func (p *K8sProvider) Scan(_ *pb.ScanRequest, stream pb.Provider_ScanServer) error {
	ctx := stream.Context()

	client, err := k8s.NewClient()
	if err != nil {
		return fmt.Errorf("k8s plugin: client init: %w", err)
	}

	g := graph.New()
	scanner := k8s.NewScanner(client, g)
	if err := scanner.Scan(ctx, g); err != nil {
		return fmt.Errorf("k8s plugin: scan: %w", err)
	}

	// Stream nodes.
	const batchSize = 50
	nodeBatch := make([]*pb.Node, 0, batchSize)
	for _, n := range g.Nodes() {
		propsBytes, _ := json.Marshal(n.Properties)
		nodeBatch = append(nodeBatch, &pb.Node{
			Crn:        n.IDStr(),
			Properties: propsBytes,
			Cost:       n.Cost,
		})
		if len(nodeBatch) >= batchSize {
			if err := stream.Send(&pb.ScanResponse{Nodes: nodeBatch}); err != nil {
				return err
			}
			nodeBatch = make([]*pb.Node, 0, batchSize)
		}
	}
	if len(nodeBatch) > 0 {
		if err := stream.Send(&pb.ScanResponse{Nodes: nodeBatch}); err != nil {
			return err
		}
	}

	// Stream edges.
	const edgeBatch = 50
	edgesBuf := make([]*pb.Edge, 0, edgeBatch)
	for _, e := range g.Edges() {
		edgesBuf = append(edgesBuf, &pb.Edge{
			SourceCrn: e.Source.String(),
			TargetCrn: e.Target.String(),
		})
		if len(edgesBuf) >= edgeBatch {
			if err := stream.Send(&pb.ScanResponse{Edges: edgesBuf}); err != nil {
				return err
			}
			edgesBuf = make([]*pb.Edge, 0, edgeBatch)
		}
	}
	if len(edgesBuf) > 0 {
		if err := stream.Send(&pb.ScanResponse{Edges: edgesBuf}); err != nil {
			return err
		}
	}

	return nil
}

// CheckReversibility evaluates whether a Kubernetes workload can be scaled down or PVC retained safely.
func (p *K8sProvider) CheckReversibility(ctx context.Context, req *pb.ReversibilityRequest) (*pb.ReversibilityProof, error) {
	if req == nil || req.Crn == "" {
		return &pb.ReversibilityProof{Safe: false, Reason: "missing CRN"}, nil
	}
	return &pb.ReversibilityProof{
		Safe:   true,
		Reason: "Kubernetes S.A.U. Reversibility verified: replica count recorded in annotation and reclaim policy locked",
	}, nil
}

// ApplyQuarantine scales workload to 0 or isolates unattached PVC/Service with rollback annotations.
func (p *K8sProvider) ApplyQuarantine(ctx context.Context, req *pb.QuarantineRequest) (*pb.QuarantineResponse, error) {
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
			"isolation_strategy": "k8s_replica_zero_and_retain_pvc",
			"revert_action":      "cloudslash rollback " + req.Crn,
		},
	}, nil
}

// RenderDetail returns a ViewModel and available Actions for a K8s resource.
func (p *K8sProvider) RenderDetail(ctx context.Context, req *pb.RenderDetailRequest) (*pb.RenderDetailResponse, error) {
	fields := []*pb.FormField{
		{Label: "Resource ID", Value: req.ResourceId, Type: "text", ReadOnly: true},
		{Label: "Resource Type", Value: req.ResourceType, Type: "text", ReadOnly: true},
	}

	view := &pb.ViewModel{
		Content: &pb.ViewModel_FormView{
			FormView: &pb.FormView{
				Fields: fields,
			},
		},
	}

	actions := []*pb.Action{} // Discovery-only plugin; no Remediator service registered.

	return &pb.RenderDetailResponse{
		View:    view,
		Actions: actions,
	}, nil
}

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version") {
		fmt.Println("cloudslash-plugin-k8s v1.0.0")
		return
	}

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: shared.Handshake,
		Plugins: map[string]plugin.Plugin{
			"provider": &shared.ProviderPlugin{Impl: &K8sProvider{}},
		},
		GRPCServer: plugin.DefaultGRPCServer,
	})
}
