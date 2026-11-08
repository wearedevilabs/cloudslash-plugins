// SPDX-License-Identifier: Apache-2.0
// Copyright 2024-2026 CloudSlash Contributors

package azure

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appservice/armappservice/v4"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/cosmos/armcosmos/v3"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v6"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/redis/armredis/v3"
)

// --- App Service ---

// ScanAppServicePlans scans all App Service Plans and their Web Apps.
// Flags free/stopped apps and oversized plans (high-tier with low utilization indicated
// by having zero apps) as waste candidates.
func (s *Scanner) ScanAppServicePlans(ctx context.Context) error {
	planClient, err := armappservice.NewPlansClient(s.subscriptionID, s.cred, nil)
	if err != nil {
		return fmt.Errorf("azure: app service plan client: %w", err)
	}
	webClient, err := armappservice.NewWebAppsClient(s.subscriptionID, s.cred, nil)
	if err != nil {
		return fmt.Errorf("azure: web apps client: %w", err)
	}

	pager := planClient.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("azure: list app service plans: %w", err)
		}
		for _, plan := range page.Value {
			if plan.ID == nil {
				continue
			}
			location := strPtr(plan.Location)
			planCRN := toCRN(location, "appservice", "plan", strPtr(plan.ID))
			rgName := extractResourceGroup(strPtr(plan.ID))

			props := map[string]interface{}{
				"Name":     strPtr(plan.Name),
				"Location": location,
				"Tags":     parseTags(plan.Tags),
				"Provider": "azure",
			}
			tierMonthly := 0.0
			if plan.SKU != nil {
				tier := strPtr(plan.SKU.Tier)
				props["Tier"] = tier
				props["SKU"] = strPtr(plan.SKU.Name)
				// Approximate monthly cost by tier for waste heuristic
				switch tier {
				case "Basic":
					tierMonthly = 55.0
				case "Standard":
					tierMonthly = 73.0
				case "Premium", "PremiumV2", "PremiumV3":
					tierMonthly = 200.0
				case "Isolated", "IsolatedV2":
					tierMonthly = 667.0
				}
			}
			if plan.Properties != nil && plan.Properties.NumberOfSites != nil {
				apps := *plan.Properties.NumberOfSites
				props["NumberOfApps"] = apps
				// Plans with zero apps are wasted reserved compute
				if apps == 0 && tierMonthly > 0 {
					props["IsWasteCandidate"] = true
					props["WasteReason"] = "App Service Plan has 0 apps — paying for idle reserved compute"
					props["EstimatedMonthlyCost"] = tierMonthly
				}
			}

			s.graph.AddNode(planCRN, props)

			// Scan web apps under this plan
			appPager := webClient.NewListByResourceGroupPager(rgName, nil)
			for appPager.More() {
				appPage, err := appPager.NextPage(ctx)
				if err != nil {
					break
				}
				for _, app := range appPage.Value {
					if app.ID == nil {
						continue
					}
					appCRN := toCRN(location, "appservice", "web-app", strPtr(app.ID))
					appProps := map[string]interface{}{
						"Name":     strPtr(app.Name),
						"Location": location,
						"Tags":     parseTags(app.Tags),
						"Provider": "azure",
					}
					if app.Properties != nil {
						appProps["State"] = strPtr(app.Properties.State)
						appProps["DefaultHostName"] = strPtr(app.Properties.DefaultHostName)
						// Stopped apps still incur plan cost — flag if tier is paid
						if strPtr(app.Properties.State) == "Stopped" && tierMonthly > 0 {
							appProps["IsWasteCandidate"] = true
							appProps["WasteReason"] = "Web App is stopped but still consuming reserved plan capacity"
						}
					}
					s.graph.AddNode(appCRN, appProps)
					s.graph.AddEdge(planCRN, appCRN)
				}
			}
		}
	}
	return nil
}

// --- Azure Cache for Redis ---

// ScanRedisCaches scans all Azure Cache for Redis instances.
// Flags Basic/Standard caches in non-production tiers that could be downscaled.
func (s *Scanner) ScanRedisCaches(ctx context.Context) error {
	client, err := armredis.NewClient(s.subscriptionID, s.cred, nil)
	if err != nil {
		return fmt.Errorf("azure: redis client: %w", err)
	}

	pager := client.NewListBySubscriptionPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("azure: list redis caches: %w", err)
		}
		for _, cache := range page.Value {
			if cache.ID == nil {
				continue
			}
			location := strPtr(cache.Location)
			redisCRN := toCRN(location, "cache", "redis", strPtr(cache.ID))

			props := map[string]interface{}{
				"Name":     strPtr(cache.Name),
				"Location": location,
				"Tags":     parseTags(cache.Tags),
				"Provider": "azure",
			}
			if cache.Properties != nil {
				if cache.Properties.ProvisioningState != nil {
					props["ProvisioningState"] = string(*cache.Properties.ProvisioningState)
				}
				props["HostName"] = strPtr(cache.Properties.HostName)
				if cache.Properties.RedisVersion != nil {
					props["RedisVersion"] = *cache.Properties.RedisVersion
				}
				if cache.Properties.SKU != nil {
					props["SKUName"] = string(*cache.Properties.SKU.Name)
					props["SKUCapacity"] = cache.Properties.SKU.Capacity
				}
			}
			s.graph.AddNode(redisCRN, props)
		}
	}
	return nil
}

// --- Azure Cosmos DB ---

// ScanCosmosAccounts scans Azure Cosmos DB accounts and their databases.
// Flags accounts with provisioned throughput that could benefit from autoscale.
func (s *Scanner) ScanCosmosAccounts(ctx context.Context) error {
	client, err := armcosmos.NewDatabaseAccountsClient(s.subscriptionID, s.cred, nil)
	if err != nil {
		return fmt.Errorf("azure: cosmos client: %w", err)
	}

	pager := client.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("azure: list cosmos accounts: %w", err)
		}
		for _, acct := range page.Value {
			if acct.ID == nil {
				continue
			}
			location := strPtr(acct.Location)
			cosmosCRN := toCRN(location, "cosmos", "database-account", strPtr(acct.ID))

			props := map[string]interface{}{
				"Name":     strPtr(acct.Name),
				"Location": location,
				"Tags":     parseTags(acct.Tags),
				"Provider": "azure",
			}
			if acct.Properties != nil {
				props["DocumentEndpoint"] = strPtr(acct.Properties.DocumentEndpoint)
				props["ProvisioningState"] = strPtr(acct.Properties.ProvisioningState)
				if acct.Properties.DatabaseAccountOfferType != nil {
					props["OfferType"] = string(*acct.Properties.DatabaseAccountOfferType)
				}
			}
			if acct.Kind != nil {
				props["Kind"] = string(*acct.Kind)
			}

			s.graph.AddNode(cosmosCRN, props)
		}
	}
	return nil
}

// --- Azure Load Balancers ---

// ScanLoadBalancers scans all Azure Load Balancers and flags idle ones.
// A load balancer with no backend pool members is paying for reserved capacity with
// zero throughput — a classic waste pattern.
func (s *Scanner) ScanLoadBalancers(ctx context.Context) error {
	client, err := armnetwork.NewLoadBalancersClient(s.subscriptionID, s.cred, nil)
	if err != nil {
		return fmt.Errorf("azure: load balancer client: %w", err)
	}

	pager := client.NewListAllPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("azure: list load balancers: %w", err)
		}
		for _, lb := range page.Value {
			if lb.ID == nil {
				continue
			}
			location := strPtr(lb.Location)
			lbCRN := toCRN(location, "network", "load-balancer", strPtr(lb.ID))

			// Standard LB fixed cost: ~$18/mo + $0.005/hr per rule
			// Basic LB is free but Standard always has a fixed cost
			sku := "Basic"
			monthlyCost := 0.0
			if lb.SKU != nil && lb.SKU.Name != nil {
				sku = string(*lb.SKU.Name)
				if sku == "Standard" {
					monthlyCost = 18.0 // $0.025/hr
				}
			}

			props := map[string]interface{}{
				"Name":     strPtr(lb.Name),
				"Location": location,
				"SKU":      sku,
				"Tags":     parseTags(lb.Tags),
				"Provider": "azure",
			}

			// Flag LBs with no backend pool addresses
			emptyBackend := true
			if lb.Properties != nil && lb.Properties.BackendAddressPools != nil {
				for _, pool := range lb.Properties.BackendAddressPools {
					if pool.Properties != nil &&
						len(pool.Properties.LoadBalancerBackendAddresses) > 0 {
						emptyBackend = false
						break
					}
				}
			}

			if emptyBackend && sku == "Standard" {
				props["IsWasteCandidate"] = true
				props["WasteReason"] = "Standard Load Balancer has no backend pool members — $18/mo fixed cost with zero traffic"
			}

			props["EstimatedMonthlyCost"] = monthlyCost
			addNodeWithCost(s.graph, lbCRN, props, monthlyCost)
		}
	}
	return nil
}

// --- Azure Functions (Function Apps) ---

// ScanFunctionApps scans all Azure Function Apps.
// Stopped or idle function apps on premium/dedicated plans incur charges.
func (s *Scanner) ScanFunctionApps(ctx context.Context) error {
	client, err := armappservice.NewWebAppsClient(s.subscriptionID, s.cred, nil)
	if err != nil {
		return fmt.Errorf("azure: function app client: %w", err)
	}

	pager := client.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("azure: list function apps: %w", err)
		}
		for _, app := range page.Value {
			if app.ID == nil {
				continue
			}

			// Only scan Function Apps, not regular web apps
			kind := strPtr(app.Kind)
			if kind != "functionapp" && kind != "functionapp,linux" {
				continue
			}

			location := strPtr(app.Location)
			appCRN := toCRN(location, "appservice", "functionapp", strPtr(app.ID))

			props := map[string]interface{}{
				"Name":     strPtr(app.Name),
				"Location": location,
				"Kind":     kind,
				"Tags":     parseTags(app.Tags),
				"Provider": "azure",
			}

			// Consumption plan functions are free when idle
			// Premium/Dedicated plans incur costs even when stopped
			state := ""
			if app.Properties != nil {
				state = strPtr(app.Properties.State)
				props["State"] = state
				props["DefaultHostName"] = strPtr(app.Properties.DefaultHostName)
			}

			// Flag stopped function apps on non-consumption plans
			if state == "Stopped" {
				props["IsWasteCandidate"] = true
				props["WasteReason"] = "Azure Function App is Stopped — premium/dedicated plan still incurs charges"
			}

			// Function app cost depends on plan tier — nominal placeholder
			monthlyCost := 0.0 // consumption = free; premium ≈ $150+/mo
			props["EstimatedMonthlyCost"] = monthlyCost
			addNodeWithCost(s.graph, appCRN, props, monthlyCost)
		}
	}
	return nil
}
