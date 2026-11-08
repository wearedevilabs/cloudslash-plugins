// SPDX-License-Identifier: Apache-2.0
// Copyright 2024-2026 CloudSlash Contributors

// Cost heuristics for GCP resources.
// Conservative estimates based on GCP public pricing (us-central1) 2026.
// Used for waste prioritisation, not billing.

package gcp

import (
	crn "github.com/DrSkyle/cloudslash-plugin-sdk/shared/crn"
	graph "github.com/DrSkyle/cloudslash-plugin-sdk/shared/scangraph"
)

// addNodeWithCost calls g.AddNode then reads the node back and sets n.Cost.
func addNodeWithCost(g *graph.Graph, id crn.CRN, props map[string]interface{}, cost float64) {
	_ = g.AddNode(id, props)
	if cost > 0 {
		if n := g.GetNode(id); n != nil {
			n.Cost = cost
		}
	}
}

// gceCostPerMonth returns approximate monthly cost for a GCE instance.
// Pricing based on Standard (non-preemptible) Linux in us-central1 (2026).
func gceCostPerMonth(machineType string) float64 {
	costs := map[string]float64{
		// E2 (general purpose)
		"e2-micro":       6.11,
		"e2-small":       12.23,
		"e2-medium":      24.46,
		"e2-standard-2":  48.92,
		"e2-standard-4":  97.83,
		"e2-standard-8":  195.67,
		"e2-standard-16": 391.33,
		"e2-standard-32": 782.67,
		// N2 (general purpose)
		"n2-standard-2":  58.11,
		"n2-standard-4":  116.22,
		"n2-standard-8":  232.44,
		"n2-standard-16": 464.89,
		"n2-standard-32": 929.77,
		// N1 (legacy general purpose)
		"n1-standard-1":  24.27,
		"n1-standard-2":  48.54,
		"n1-standard-4":  97.09,
		"n1-standard-8":  194.18,
		"n1-standard-16": 388.36,
		"n1-standard-32": 776.71,
		// C2 (compute optimised)
		"c2-standard-4":  134.57,
		"c2-standard-8":  269.14,
		"c2-standard-16": 538.29,
		"c2-standard-30": 1009.28,
		// M2 (memory optimised)
		"m2-ultramem-208": 14348.08,
		"m2-megamem-416":  28696.16,
	}
	if c, ok := costs[machineType]; ok {
		return c
	}
	// Default heuristic for unknown types (~n1-standard-2 equivalent)
	return 48.54
}

// diskCostPerMonth returns approximate monthly cost for a GCP persistent disk.
// Pricing: Standard PD ~$0.040/GB, SSD PD ~$0.170/GB, Balanced ~$0.100/GB.
func gcpDiskCostPerMonth(sizeGB int64, diskType string) float64 {
	pricePerGB := 0.040 // Standard PD
	switch diskType {
	case "pd-ssd":
		pricePerGB = 0.170
	case "pd-balanced":
		pricePerGB = 0.100
	case "pd-extreme":
		pricePerGB = 0.220
	}
	return float64(sizeGB) * pricePerGB
}

// gkeCostPerMonth returns approximate monthly cost for a GKE cluster.
// GKE control plane: $74.40/mo for standard; node costs by count and type.
func gkeCostPerMonth(nodeCount int64, autopilot bool) float64 {
	if autopilot {
		// Autopilot: $0.10/vCPU-hr — assume ~2 vCPU average
		return float64(nodeCount) * 144.0
	}
	// Standard: control plane ($74.40 for > 5 nodes) + ~$58/node (n2-standard-2)
	controlplane := 74.40
	if nodeCount <= 5 {
		controlplane = 0 // Free tier
	}
	return controlplane + float64(nodeCount)*58.11
}

// sqlCostPerMonth returns approximate monthly cost for a Cloud SQL instance.
func gcloudSQLCostPerMonth(tier string) float64 {
	costs := map[string]float64{
		"db-f1-micro":      7.66,
		"db-g1-small":      25.55,
		"db-n1-standard-1": 51.09,
		"db-n1-standard-2": 102.18,
		"db-n1-standard-4": 204.36,
		"db-n1-standard-8": 408.73,
		"db-n1-highmem-2":  131.36,
		"db-n1-highmem-4":  262.72,
		"db-n1-highmem-8":  525.44,
		"db-n1-highmem-16": 1050.87,
	}
	if c, ok := costs[tier]; ok {
		return c
	}
	return 51.09 // Default: db-n1-standard-1
}

// cloudRunCostPerMonth returns approximate monthly cost for a Cloud Run service.
// Based on request/CPU allocation model at typical usage.
func cloudRunCostPerMonth(requestsPerDay int64) float64 {
	if requestsPerDay == 0 {
		return 0.0 // Truly idle: zero cost
	}
	// ~$0.4/million requests + ~$0.0000024/vCPU-second
	costPerMillionReq := 0.40
	monthlyReqs := float64(requestsPerDay) * 30 / 1e6
	return monthlyReqs * costPerMillionReq
}
