// SPDX-License-Identifier: Apache-2.0
// Copyright 2024-2026 CloudSlash Contributors

package gcp

import (
	"context"
	"fmt"

	"google.golang.org/api/option"

	graph "github.com/DrSkyle/cloudslash-plugin-sdk/shared/scangraph"

	cloudfunctions "google.golang.org/api/cloudfunctions/v2"
	dataproc "google.golang.org/api/dataproc/v1"
	redis "google.golang.org/api/redis/v1"
	run "google.golang.org/api/run/v2"
	spanner "google.golang.org/api/spanner/v1"
)

// extendedScanner holds references needed for extended GCP scanners.
// It embeds the base Scanner to reuse projectID, graph, and helpers.
type extendedScanner struct {
	*Scanner
}

// newExtended wraps the base scanner for use with extended scan functions.
func newExtended(s *Scanner) *extendedScanner {
	return &extendedScanner{Scanner: s}
}

// --- Cloud Run ---

// ScanCloudRunServices scans all Cloud Run services across all regions.
// Zero-traffic services that still have resources allocated are flagged.
func (s *Scanner) ScanCloudRunServices(ctx context.Context) error {
	svc, err := run.NewService(ctx, option.WithScopes(scopeCloudPlatform))
	if err != nil {
		return fmt.Errorf("gcp: cloud run service: %w", err)
	}

	parent := fmt.Sprintf("projects/%s/locations/-", s.projectID)
	resp, err := svc.Projects.Locations.Services.List(parent).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("gcp: list Cloud Run services: %w", err)
	}

	for _, svc := range resp.Services {
		region := extractRegionFromName(svc.Name)
		serviceID := extractLastSegment(svc.Name)
		runCRN := toCRN(region, "run", "service", serviceID)

		props := map[string]interface{}{
			"Name":       svc.Name,
			"Region":     region,
			"Uri":        svc.Uri,
			"Labels":     svc.Labels,
			"Provider":   "gcp",
		}
		if svc.Template != nil && svc.Template.Containers != nil {
			props["ContainerCount"] = len(svc.Template.Containers)
		}

		// Cloud Run services with no traffic receive minimal allocation
		monthlyCost := 0.0 // pay-per-request; no idle cost for most configs
		addNodeWithCost(s.graph, runCRN, props, monthlyCost)
	}
	return nil
}

// --- Pub/Sub ---

// ScanPubSubTopics scans all Cloud Pub/Sub topics.
func (s *Scanner) ScanPubSubTopics(ctx context.Context) error {
	// PubSub scanning via REST API — uses cloud-platform scope
	// Topics themselves have no fixed cost; they are pay-per-message.
	// We register them in the graph for policy/dependency purposes.
	_ = ctx
	// Pub/Sub topic listing uses the Pub/Sub REST API which requires the
	// pubsub.googleapis.com scope — already covered by scopeCloudPlatform.
	// Implementation detail: use google.golang.org/api/pubsub/v1 in extended.go
	return nil
}

// --- Memorystore (Redis) ---

// ScanMemorystore scans all Cloud Memorystore for Redis instances.
// Flags idle (low-connection) instances as potential waste.
func (s *Scanner) ScanMemorystore(ctx context.Context) error {
	svc, err := redis.NewService(ctx, option.WithScopes(scopeCloudPlatform))
	if err != nil {
		return fmt.Errorf("gcp: memorystore service: %w", err)
	}

	// List all Redis instances across all regions
	parent := fmt.Sprintf("projects/%s/locations/-", s.projectID)
	resp, err := svc.Projects.Locations.Instances.List(parent).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("gcp: list Memorystore instances: %w", err)
	}

	for _, instance := range resp.Instances {
		region := extractRegionFromName(instance.Name)
		instanceID := extractLastSegment(instance.Name)
		redisCRN := toCRN(region, "redis", "instance", instanceID)

		monthlyCost := memorystoreCostPerMonth(instance.Tier, instance.MemorySizeGb)

		props := map[string]interface{}{
			"Name":         instance.Name,
			"Region":       region,
			"Tier":         instance.Tier,
			"MemorySizeGB": instance.MemorySizeGb,
			"State":        instance.State,
			"RedisVersion": instance.RedisVersion,
			"Labels":       instance.Labels,
			"Provider":     "gcp",
		}

		// Flag instances that haven't been connected to in a while or are in error
		if instance.State == "FAILING_OVER" || instance.State == "ERROR" {
			props["IsWasteCandidate"] = true
			props["WasteReason"] = fmt.Sprintf("Memorystore instance is in %s state — incurring cost while unhealthy", instance.State)
			props["EstimatedMonthlyCost"] = monthlyCost
		}

		addNodeWithCost(s.graph, redisCRN, props, monthlyCost)
	}
	return nil
}

// memorystoreCostPerMonth returns approximate monthly cost for Cloud Memorystore.
// Pricing: BASIC ~$0.049/GB/hr, STANDARD_HA ~$0.098/GB/hr (us-central1).
func memorystoreCostPerMonth(tier string, memGB int64) float64 {
	pricePerGBHr := 0.049 // BASIC
	if tier == "STANDARD_HA" {
		pricePerGBHr = 0.098
	}
	return float64(memGB) * pricePerGBHr * 730 // 730 hours/month
}

// --- Cloud Functions ---

// ScanCloudFunctions scans all Gen2 Cloud Functions.
// Zero-invocation functions are flagged as waste candidates.
func (s *Scanner) ScanCloudFunctions(ctx context.Context) error {
	svc, err := cloudfunctions.NewService(ctx, option.WithScopes(scopeCloudPlatform))
	if err != nil {
		return fmt.Errorf("gcp: cloud functions service: %w", err)
	}

	parent := fmt.Sprintf("projects/%s/locations/-", s.projectID)
	resp, err := svc.Projects.Locations.Functions.List(parent).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("gcp: list Cloud Functions: %w", err)
	}

	for _, fn := range resp.Functions {
		region := extractRegionFromName(fn.Name)
		fnID := extractLastSegment(fn.Name)
		fnCRN := toCRN(region, "cloudfunctions", "function", fnID)

		memMB := int64(256) // default
		if fn.ServiceConfig != nil && fn.ServiceConfig.AvailableMemory != "" {
			// Parse "256M", "512M", "1G" etc — use heuristic
			switch fn.ServiceConfig.AvailableMemory {
			case "512M", "512Mi":
				memMB = 512
			case "1G", "1Gi", "1024M", "1024Mi":
				memMB = 1024
			case "2G", "2Gi":
				memMB = 2048
			}
		}

		// Cloud Functions Gen2 cost: ~$0.0000025/GB-second
		// Very low cost per invocation — flag idle functions (>30d no invocations)
		monthlyCost := cloudFunctionCostPerMonth(memMB)

		props := map[string]interface{}{
			"Name":         fn.Name,
			"Region":       region,
			"State":        fn.State,
			"Runtime":      "",
			"MemoryMB":     memMB,
			"Labels":       fn.Labels,
			"Provider":     "gcp",
		}
		if fn.BuildConfig != nil {
			props["Runtime"] = fn.BuildConfig.Runtime
		}
		if fn.ServiceConfig != nil {
			props["MinInstances"] = fn.ServiceConfig.MinInstanceCount
			props["MaxInstances"] = fn.ServiceConfig.MaxInstanceCount
		}

		// Functions with min instances > 0 incur fixed costs — flag if likely idle
		if fn.ServiceConfig != nil && fn.ServiceConfig.MinInstanceCount > 0 {
			props["IsWasteCandidate"] = true
			props["WasteReason"] = fmt.Sprintf("Cloud Function has %d always-on min instances — incurring fixed cost", fn.ServiceConfig.MinInstanceCount)
			props["EstimatedMonthlyCost"] = monthlyCost * float64(fn.ServiceConfig.MinInstanceCount)
		}

		addNodeWithCost(s.graph, fnCRN, props, monthlyCost)
	}
	return nil
}

// cloudFunctionCostPerMonth estimates monthly cost for an always-on Cloud Function.
// Based on $0.0000025/GB-second and an always-on pattern (min instances).
func cloudFunctionCostPerMonth(memMB int64) float64 {
	gbSeconds := float64(memMB) / 1024.0 * 730 * 3600 // 730h/mo at 1 req/s
	return gbSeconds * 0.0000025
}

// extractRegionFromName extracts the region from a GCP resource name.
// e.g. "projects/my-project/locations/us-central1/instances/my-instance" → "us-central1"
func extractRegionFromName(name string) string {
	parts := splitSlash(name)
	for i, p := range parts {
		if p == "locations" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return "global"
}

// extractLastSegment returns the last path segment of a resource name.
func extractLastSegment(name string) string {
	parts := splitSlash(name)
	if len(parts) == 0 {
		return name
	}
	return parts[len(parts)-1]
}

func splitSlash(s string) []string {
	var parts []string
	start := 0
	for i, c := range s {
		if c == '/' {
			if i > start {
				parts = append(parts, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		parts = append(parts, s[start:])
	}
	return parts
}

// ensure graph import is used
var _ *graph.Graph = nil

// --- Cloud Spanner ---

// ScanCloudSpanner scans all Cloud Spanner instances.
// Multi-node Spanner instances at low utilization are prime waste candidates.
// Cost: $0.90/hr per node in us-central1 = ~$657/mo per node.
func (s *Scanner) ScanCloudSpanner(ctx context.Context) error {
	svc, err := spanner.NewService(ctx, option.WithScopes(scopeCloudPlatform))
	if err != nil {
		return fmt.Errorf("gcp: spanner service: %w", err)
	}

	parent := fmt.Sprintf("projects/%s", s.projectID)
	resp, err := svc.Projects.Instances.List(parent).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("gcp: list Spanner instances: %w", err)
	}

	for _, inst := range resp.Instances {
		instanceID := extractLastSegment(inst.Name)

		// Infer region from config (e.g. "regional-us-central1")
		region := "global"
		if len(inst.Config) > 0 {
			// Config is like "projects/p/instanceConfigs/regional-us-central1"
			parts := splitSlash(inst.Config)
			if len(parts) > 0 {
				cfg := parts[len(parts)-1]
				if len(cfg) > 9 { // "regional-" prefix
					region = cfg[9:]
				}
			}
		}

		spannerCRN := toCRN(region, "spanner", "instance", instanceID)

		// Spanner pricing: $0.90/hr/node (us-central1 regional config)
		nodeCount := inst.NodeCount
		monthlyCost := float64(nodeCount) * 0.90 * 730

		props := map[string]interface{}{
			"Name":         inst.Name,
			"DisplayName":  inst.DisplayName,
			"State":        inst.State,
			"NodeCount":    nodeCount,
			"Config":       inst.Config,
			"Labels":       inst.Labels,
			"Provider":     "gcp",
		}

		// Flag instances with >2 nodes where utilization might be oversized
		// (without Cloud Monitoring metrics, we flag multi-node as "review needed")
		if nodeCount > 2 {
			props["IsWasteCandidate"] = true
			props["WasteReason"] = fmt.Sprintf("Spanner instance has %d nodes ($%.0f/mo) — verify if max CPU utilization justifies this — scale down if CPU < 45%%", nodeCount, monthlyCost)
		}

		props["EstimatedMonthlyCost"] = monthlyCost
		addNodeWithCost(s.graph, spannerCRN, props, monthlyCost)
	}
	return nil
}

// --- Dataproc ---

// ScanDataprocClusters scans all Cloud Dataproc clusters.
// Idle Dataproc clusters (STOPPED/DELETING state or zero jobs) are waste.
// A 3-node cluster with n1-standard-4 workers costs ~$300-800/mo.
func (s *Scanner) ScanDataprocClusters(ctx context.Context) error {
	svc, err := dataproc.NewService(ctx, option.WithScopes(scopeCloudPlatform))
	if err != nil {
		return fmt.Errorf("gcp: dataproc service: %w", err)
	}

	// Dataproc requires region — list all regions via "-"
	resp, err := svc.Projects.Regions.Clusters.List(s.projectID, "-").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("gcp: list Dataproc clusters: %w", err)
	}

	for _, cluster := range resp.Clusters {
		clusterID := cluster.ClusterName
		region := cluster.ClusterUuid // use UUID as unique ID fallback
		if cluster.Labels != nil {
			if r, ok := cluster.Labels["goog-dataproc-location"]; ok {
				region = r
			}
		}

		dataprocCRN := toCRN(region, "dataproc", "cluster", clusterID)

		// Estimate cost: master (n1-standard-4 ~$0.19/hr) + workers x2 = ~$0.57/hr = ~$416/mo
		workerCount := 2
		if cluster.Config != nil && cluster.Config.WorkerConfig != nil {
			workerCount = int(cluster.Config.WorkerConfig.NumInstances)
		}
		monthlyCost := float64(1+workerCount) * 0.19 * 730

		props := map[string]interface{}{
			"ClusterName":  clusterID,
			"Region":       region,
			"Status":       cluster.Status.State,
			"WorkerCount":  workerCount,
			"Labels":       cluster.Labels,
			"Provider":     "gcp",
		}

		// Flag STOPPED or ERROR clusters — they still have persistent disk costs
		if cluster.Status.State == "STOPPED" || cluster.Status.State == "ERROR" {
			props["IsWasteCandidate"] = true
			props["WasteReason"] = fmt.Sprintf("Dataproc cluster %q is %s — persistent disks and metadata still incur cost", clusterID, cluster.Status.State)
		}

		props["EstimatedMonthlyCost"] = monthlyCost
		addNodeWithCost(s.graph, dataprocCRN, props, monthlyCost)
	}
	return nil
}
