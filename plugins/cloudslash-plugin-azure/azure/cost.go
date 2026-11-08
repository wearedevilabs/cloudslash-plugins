// SPDX-License-Identifier: Apache-2.0
// Copyright 2024-2026 CloudSlash Contributors

// Cost heuristics for Azure resources.
// These are conservative estimates based on Azure public pricing (West US 2)
// updated for 2026. Actual costs depend on region, reserved pricing, and
// customer agreements. Used for waste prioritisation, not billing.

package azure

import (
	crn "github.com/DrSkyle/cloudslash-plugin-sdk/shared/crn"
	graph "github.com/DrSkyle/cloudslash-plugin-sdk/shared/scangraph"
)

// addNodeWithCost is a helper that calls g.AddNode then reads the node back
// and sets n.Cost, so the cost flows through to pb.Node.Cost in the plugin.
func addNodeWithCost(g *graph.Graph, id crn.CRN, props map[string]interface{}, cost float64) {
	_ = g.AddNode(id, props)
	if cost > 0 {
		if n := g.GetNode(id); n != nil {
			n.Cost = cost
		}
	}
}

// vmCostPerMonth returns an approximate monthly cost for a given Azure VM size.
// Pricing based on pay-as-you-go Linux rates in East US (2026).
func vmCostPerMonth(vmSize string) float64 {
	costs := map[string]float64{
		// General Purpose
		"Standard_B1s":   7.59,
		"Standard_B2s":   30.37,
		"Standard_B4ms":  60.74,
		"Standard_D2s_v3": 69.35,
		"Standard_D4s_v3": 138.70,
		"Standard_D8s_v3": 277.40,
		"Standard_D16s_v3": 554.80,
		"Standard_D32s_v3": 1109.60,
		// Compute Optimised
		"Standard_F2s_v2": 61.46,
		"Standard_F4s_v2": 122.92,
		"Standard_F8s_v2": 245.84,
		"Standard_F16s_v2": 491.68,
		// Memory Optimised
		"Standard_E4s_v3": 200.16,
		"Standard_E8s_v3": 400.32,
		"Standard_E16s_v3": 800.64,
		"Standard_E32s_v3": 1601.28,
		// High performance
		"Standard_M8ms":  1228.90,
		"Standard_M64ms": 9831.00,
	}
	if c, ok := costs[vmSize]; ok {
		return c
	}
	// Default heuristic for unknown sizes (~D4s_v3 middle ground)
	return 100.0
}

// diskCostPerMonth returns approximate monthly cost for a managed disk.
// Based on Premium SSD pricing in East US (2026).
func diskCostPerMonth(sizeGB int32, diskTier string) float64 {
	// Standard SSD: ~$0.076/GB/mo, Premium SSD: ~$0.135/GB/mo
	pricePerGB := 0.076
	if diskTier == "Premium_LRS" || diskTier == "Premium_ZRS" {
		pricePerGB = 0.135
	}
	if diskTier == "UltraSSD_LRS" {
		pricePerGB = 0.29
	}
	return float64(sizeGB) * pricePerGB
}

// appServicePlanCostPerMonth returns the monthly cost for an App Service Plan tier.
func appServicePlanCostPerMonth(tier string) float64 {
	switch tier {
	case "Basic":
		return 55.0
	case "Standard":
		return 73.0
	case "Premium", "PremiumV2":
		return 150.0
	case "PremiumV3":
		return 200.0
	case "Isolated", "IsolatedV2":
		return 667.0
	default:
		return 0.0
	}
}

// sqlDatabaseCostPerMonth returns approximate monthly cost for Azure SQL Database.
func sqlDatabaseCostPerMonth(edition string) float64 {
	switch edition {
	case "Basic":
		return 4.90
	case "Standard":
		return 15.00
	case "Premium":
		return 116.00
	case "GeneralPurpose":
		return 370.00
	case "BusinessCritical":
		return 1100.00
	default:
		return 50.0
	}
}

// aksCostPerMonth returns approximate monthly cost for an AKS cluster node pool.
// AKS control plane is free; this covers default node pool cost.
func aksCostPerMonth(nodeCount int) float64 {
	if nodeCount <= 0 {
		return 0
	}
	// Assume Standard_D2s_v3 default node: ~$69/mo per node
	return float64(nodeCount) * 69.35
}

// redisCostPerMonth returns approximate monthly cost for Azure Cache for Redis.
func redisCostPerMonth(skuName, capacity string) float64 {
	// C0 Basic ~$16, C1 ~$55, P1 Premium ~$310
	switch skuName + "_" + capacity {
	case "Basic_0":
		return 16.0
	case "Basic_1":
		return 55.0
	case "Standard_0":
		return 55.0
	case "Standard_1":
		return 110.0
	case "Premium_1":
		return 310.0
	case "Premium_2":
		return 620.0
	default:
		return 55.0
	}
}
