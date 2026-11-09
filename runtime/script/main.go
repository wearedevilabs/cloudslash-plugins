// SPDX-License-Identifier: Apache-2.0
// Copyright 2024-2026 CloudSlash Contributors

// Script plugin runtime: lets anyone write a CloudSlash plugin as a plain
// script (Python, Bash, Node, ...). The runtime sits next to a scan script
// (scan.py, scan.sh, scan.js or scan) and manifest.yaml, runs it, and turns
// its JSON output into CloudSlash resources.
//
// Script contract: print {"provider":"x","account":"a","resources":[{"id":"..",
// "type":"disk","service":"storage","region":"us-east-1","cost_monthly":12.5,
// "tags":{"k":"v"}, ...any extra fields}]} to stdout; exit 0.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/DrSkyle/cloudslash-plugin-sdk/shared"
	"github.com/DrSkyle/cloudslash-plugin-sdk/shared/crn"
	pb "github.com/DrSkyle/cloudslash-plugin-sdk/shared/proto"
	"github.com/hashicorp/go-plugin"
)

type Provider struct{ pb.UnimplementedProviderServer }

func pluginDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

func findScript() (string, []string, error) {
	dir := pluginDir()
	for _, c := range []struct{ file, runner string }{{"scan.py", "python3"}, {"scan.sh", "bash"}, {"scan.js", "node"}, {"scan", ""}} {
		p := filepath.Join(dir, c.file)
		if _, err := os.Stat(p); err == nil {
			if c.runner == "" {
				return p, nil, nil
			}
			return c.runner, []string{p}, nil
		}
	}
	return "", nil, fmt.Errorf("no scan script (scan.py, scan.sh, scan.js, scan) found next to the plugin in %s", dir)
}

type scriptResource struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"`
	Service     string            `json:"service"`
	Region      string            `json:"region"`
	CostMonthly float64           `json:"cost_monthly"`
	Tags        map[string]string `json:"tags"`
	Extra       map[string]any    `json:"properties"`
}

type scriptOutput struct {
	Provider  string           `json:"provider"`
	Account   string           `json:"account"`
	Resources []scriptResource `json:"resources"`
}

func (p *Provider) Handshake(ctx context.Context, req *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
	return &pb.HandshakeResponse{Compatible: true, PluginVersion: "1.0.0"}, nil
}

func (p *Provider) Scan(req *pb.ScanRequest, stream pb.Provider_ScanServer) error {
	bin, args, err := findScript()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(stream.Context(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = pluginDir()
	cmd.Env = os.Environ()
	if req != nil {
		for k, v := range req.Config {
			cmd.Env = append(cmd.Env, "CLOUDSLASH_"+strings.ToUpper(k)+"="+v)
		}
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("scan script failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var parsed scriptOutput
	if err := json.Unmarshal(out, &parsed); err != nil {
		return fmt.Errorf("scan script must print JSON: %w", err)
	}
	provider := strings.ToLower(strings.TrimSpace(parsed.Provider))
	if provider == "" {
		provider = "script"
	}
	account := strings.TrimSpace(parsed.Account)
	if account == "" {
		account = "local"
	}
	batch := &pb.ScanResponse{}
	seen := map[string]bool{}
	for _, r := range parsed.Resources {
		if r.ID == "" || r.Type == "" {
			return errors.New("every resource needs an id and a type")
		}
		service, region := r.Service, r.Region
		if service == "" {
			service = "resource"
		}
		if region == "" {
			region = "global"
		}
		id := crn.New(provider, region, service, r.Type, account, r.ID).String()
		if _, err := crn.Parse(id); err != nil {
			return fmt.Errorf("invalid resource identity %q: %w", r.ID, err)
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		props := map[string]any{}
		for k, v := range r.Extra {
			props[k] = v
		}
		props["tags"] = r.Tags
		props["cost_monthly"] = r.CostMonthly
		props["cost_known"] = r.CostMonthly > 0
		enc, err := json.Marshal(props)
		if err != nil {
			return err
		}
		batch.Nodes = append(batch.Nodes, &pb.Node{Crn: id, Properties: enc})
	}
	if len(batch.Nodes) == 0 {
		return nil
	}
	return stream.Send(batch)
}

func (p *Provider) ApplyQuarantine(ctx context.Context, req *pb.QuarantineRequest) (*pb.QuarantineResponse, error) {
	return &pb.QuarantineResponse{Success: false}, errors.New("script plugins are read-only")
}

func (p *Provider) CheckReversibility(ctx context.Context, req *pb.ReversibilityRequest) (*pb.ReversibilityProof, error) {
	return &pb.ReversibilityProof{Safe: false, Reason: "read-only discovery does not establish action reversibility"}, nil
}

func (p *Provider) GetMetadata(context.Context, *pb.GetMetadataRequest) (*pb.GetMetadataResponse, error) {
	return &pb.GetMetadataResponse{PluginName: "cloudslash-plugin-script", PluginVersion: "1.0.0", ProviderName: "script",
		Capabilities: []*pb.Capability{{Name: "discovery", Version: "1.0.0", Available: true}}}, nil
}

func (p *Provider) PreflightCheck(ctx context.Context, _ *pb.PreflightRequest) (*pb.PreflightResponse, error) {
	if _, _, err := findScript(); err != nil {
		return &pb.PreflightResponse{Provider: "script", Error: err.Error()}, nil
	}
	return &pb.PreflightResponse{Provider: "script", Ok: true}, nil
}

func main() {
	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: shared.Handshake,
		Plugins:         map[string]plugin.Plugin{"provider": &shared.ProviderPlugin{Impl: &Provider{}}},
		GRPCServer:      plugin.DefaultGRPCServer,
	})
}
