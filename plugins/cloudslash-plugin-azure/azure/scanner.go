// SPDX-License-Identifier: Apache-2.0
// Copyright 2024-2026 CloudSlash Contributors

package azure

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v6"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v6"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/sql/armsql"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	graph "github.com/DrSkyle/cloudslash-plugin-sdk/shared/scangraph"
)

// Scanner holds Azure API clients and the local graph for scan results.
type Scanner struct {
	cred           *azidentity.DefaultAzureCredential
	subscriptionID string
	graph          *graph.Graph
}

// NewScanner creates an Azure scanner using DefaultAzureCredential.
// Credentials resolve in order: env vars → managed identity → CLI login.
func NewScanner(subscriptionID string, g *graph.Graph) (*Scanner, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("azure: credential init: %w", err)
	}
	return &Scanner{
		cred:           cred,
		subscriptionID: subscriptionID,
		graph:          g,
	}, nil
}

// ScanVirtualMachines scans all VMs across all resource groups.
// Cost is estimated from VM size using the production cost table in cost.go.
func (s *Scanner) ScanVirtualMachines(ctx context.Context) error {
	client, err := armcompute.NewVirtualMachinesClient(s.subscriptionID, s.cred, nil)
	if err != nil {
		return fmt.Errorf("azure: vm client: %w", err)
	}

	pager := client.NewListAllPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("azure: list VMs: %w", err)
		}
		for _, vm := range page.Value {
			if vm.ID == nil {
				continue
			}
			location := strPtr(vm.Location)
			vmCRN := toCRN(location, "compute", "virtual-machine", strPtr(vm.ID))

			vmSize := ""
			props := map[string]interface{}{
				"Name":              strPtr(vm.Name),
				"Location":         location,
				"ProvisioningState": strPtr(vm.Properties.ProvisioningState),
				"Tags":             parseTags(vm.Tags),
				"Provider":         "azure",
			}
			if vm.Properties.HardwareProfile != nil {
				vmSize = string(*vm.Properties.HardwareProfile.VMSize)
				props["VMSize"] = vmSize
			}
			if vm.Properties.StorageProfile != nil && vm.Properties.StorageProfile.OSDisk != nil {
				if vm.Properties.StorageProfile.OSDisk.DiskSizeGB != nil {
					props["OSDiskSizeGB"] = *vm.Properties.StorageProfile.OSDisk.DiskSizeGB
				}
			}

			monthlyCost := vmCostPerMonth(vmSize)
			props["EstimatedMonthlyCost"] = monthlyCost
			addNodeWithCost(s.graph, vmCRN, props, monthlyCost)
		}
	}
	return nil
}

// ScanManagedDisks scans all managed disks and flags unattached ones as waste candidates.
func (s *Scanner) ScanManagedDisks(ctx context.Context) error {
	client, err := armcompute.NewDisksClient(s.subscriptionID, s.cred, nil)
	if err != nil {
		return fmt.Errorf("azure: disk client: %w", err)
	}

	pager := client.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("azure: list disks: %w", err)
		}
		for _, disk := range page.Value {
			if disk.ID == nil {
				continue
			}
			location := strPtr(disk.Location)
			diskCRN := toCRN(location, "compute", "disk", strPtr(disk.ID))

			sizeGB := int32(0)
			diskTier := ""
			props := map[string]interface{}{
				"Name":     strPtr(disk.Name),
				"Location": location,
				"Tags":     parseTags(disk.Tags),
				"Provider": "azure",
			}
			if disk.Properties != nil {
				props["DiskState"] = string(*disk.Properties.DiskState)
				if disk.Properties.DiskSizeGB != nil {
					sizeGB = *disk.Properties.DiskSizeGB
					props["DiskSizeGB"] = sizeGB
				}
				// Unattached disks are waste candidates
				if disk.Properties.DiskState != nil && *disk.Properties.DiskState == armcompute.DiskStateUnattached {
					props["IsWasteCandidate"] = true
					props["WasteReason"] = "managed disk is unattached — not associated with any VM"
				}
			}
			if disk.SKU != nil && disk.SKU.Name != nil {
				diskTier = string(*disk.SKU.Name)
				props["SKU"] = diskTier
			}

			monthlyCost := diskCostPerMonth(sizeGB, diskTier)
			props["EstimatedMonthlyCost"] = monthlyCost
			addNodeWithCost(s.graph, diskCRN, props, monthlyCost)
		}
	}
	return nil
}

// ScanPublicIPs scans all public IP addresses and flags unassociated ones.
// An unassociated static IP costs ~$3.65/mo — waste with zero value.
func (s *Scanner) ScanPublicIPs(ctx context.Context) error {
	client, err := armnetwork.NewPublicIPAddressesClient(s.subscriptionID, s.cred, nil)
	if err != nil {
		return fmt.Errorf("azure: public IP client: %w", err)
	}

	pager := client.NewListAllPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("azure: list public IPs: %w", err)
		}
		for _, pip := range page.Value {
			if pip.ID == nil {
				continue
			}
			location := strPtr(pip.Location)
			pipCRN := toCRN(location, "network", "public-ip", strPtr(pip.ID))

			// Static public IP cost: $0.005/hr = ~$3.65/mo
			monthlyCost := 3.65
			props := map[string]interface{}{
				"Name":                 strPtr(pip.Name),
				"Location":             location,
				"Tags":                 parseTags(pip.Tags),
				"Provider":             "azure",
				"EstimatedMonthlyCost": monthlyCost,
			}
			if pip.Properties != nil {
				props["IPAddress"] = strPtr(pip.Properties.IPAddress)
				props["AllocationMethod"] = string(*pip.Properties.PublicIPAllocationMethod)
				// Flag unassociated static IPs as waste candidates
				if pip.Properties.IPConfiguration == nil {
					props["IsWasteCandidate"] = true
					props["WasteReason"] = "static public IP is unassociated — idle IP incurs hourly charges"
				}
			}

			addNodeWithCost(s.graph, pipCRN, props, monthlyCost)
		}
	}
	return nil
}

// ScanSQLDatabases scans all Azure SQL servers and databases.
func (s *Scanner) ScanSQLDatabases(ctx context.Context) error {
	serverClient, err := armsql.NewServersClient(s.subscriptionID, s.cred, nil)
	if err != nil {
		return fmt.Errorf("azure: sql server client: %w", err)
	}
	dbClient, err := armsql.NewDatabasesClient(s.subscriptionID, s.cred, nil)
	if err != nil {
		return fmt.Errorf("azure: sql db client: %w", err)
	}

	serverPager := serverClient.NewListPager(nil)
	for serverPager.More() {
		serverPage, err := serverPager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("azure: list SQL servers: %w", err)
		}
		for _, server := range serverPage.Value {
			if server.ID == nil || server.Name == nil {
				continue
			}
			location := strPtr(server.Location)
			serverCRN := toCRN(location, "sql", "server", strPtr(server.ID))
			rgName := extractResourceGroup(strPtr(server.ID))

			serverProps := map[string]interface{}{
				"Name":     strPtr(server.Name),
				"Location": location,
				"Tags":     parseTags(server.Tags),
				"Provider": "azure",
			}
			// SQL Server itself is free — cost is for databases
			addNodeWithCost(s.graph, serverCRN, serverProps, 0)

			// Scan databases on this server
			dbPager := dbClient.NewListByServerPager(rgName, strPtr(server.Name), nil)
			for dbPager.More() {
				dbPage, err := dbPager.NextPage(ctx)
				if err != nil {
					break
				}
				for _, db := range dbPage.Value {
					if db.ID == nil {
						continue
					}
					dbCRN := toCRN(location, "sql", "database", strPtr(db.ID))

					edition := ""
					dbProps := map[string]interface{}{
						"Name":     strPtr(db.Name),
						"Location": location,
						"Tags":     parseTags(db.Tags),
						"Provider": "azure",
					}
					if db.Properties != nil {
						dbProps["Status"] = string(*db.Properties.Status)
						if db.SKU != nil {
							dbProps["SKU"] = strPtr(db.SKU.Name)
							if db.SKU.Tier != nil {
								edition = *db.SKU.Tier
								dbProps["Edition"] = edition
							}
						}
						// Flag paused databases as waste candidates
						if db.Properties.Status != nil && *db.Properties.Status == armsql.DatabaseStatusPaused {
							dbProps["IsWasteCandidate"] = true
							dbProps["WasteReason"] = "Azure SQL database is paused — still incurring storage costs"
						}
					}

					monthlyCost := sqlDatabaseCostPerMonth(edition)
					dbProps["EstimatedMonthlyCost"] = monthlyCost
					addNodeWithCost(s.graph, dbCRN, dbProps, monthlyCost)
					s.graph.AddEdge(serverCRN, dbCRN) //nolint:errcheck
				}
			}
		}
	}
	return nil
}

// ScanAKSClusters scans all Azure Kubernetes Service clusters.
func (s *Scanner) ScanAKSClusters(ctx context.Context) error {
	client, err := armcontainerservice.NewManagedClustersClient(s.subscriptionID, s.cred, nil)
	if err != nil {
		return fmt.Errorf("azure: AKS client: %w", err)
	}

	pager := client.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("azure: list AKS clusters: %w", err)
		}
		for _, cluster := range page.Value {
			if cluster.ID == nil {
				continue
			}
			location := strPtr(cluster.Location)
			clusterCRN := toCRN(location, "containerservice", "managed-cluster", strPtr(cluster.ID))

			nodeCount := 0
			props := map[string]interface{}{
				"Name":     strPtr(cluster.Name),
				"Location": location,
				"Tags":     parseTags(cluster.Tags),
				"Provider": "azure",
			}
			if cluster.Properties != nil {
				props["ProvisioningState"] = strPtr(cluster.Properties.ProvisioningState)
				props["KubernetesVersion"] = strPtr(cluster.Properties.KubernetesVersion)
				if cluster.Properties.AgentPoolProfiles != nil {
					total := int32(0)
					for _, pool := range cluster.Properties.AgentPoolProfiles {
						if pool.Count != nil {
							total += *pool.Count
						}
					}
					nodeCount = int(total)
					props["TotalNodes"] = nodeCount
				}
			}

			monthlyCost := aksCostPerMonth(nodeCount)
			props["EstimatedMonthlyCost"] = monthlyCost
			addNodeWithCost(s.graph, clusterCRN, props, monthlyCost)
		}
	}
	return nil
}

// ScanStorageAccounts scans Azure Storage Accounts.
// Storage cost is usage-dependent; we record a nominal $5/mo placeholder.
func (s *Scanner) ScanStorageAccounts(ctx context.Context) error {
	client, err := armstorage.NewAccountsClient(s.subscriptionID, s.cred, nil)
	if err != nil {
		return fmt.Errorf("azure: storage client: %w", err)
	}

	pager := client.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("azure: list storage accounts: %w", err)
		}
		for _, acct := range page.Value {
			if acct.ID == nil {
				continue
			}
			location := strPtr(acct.Location)
			acctCRN := toCRN(location, "storage", "storage-account", strPtr(acct.ID))

			props := map[string]interface{}{
				"Name":     strPtr(acct.Name),
				"Location": location,
				"Tags":     parseTags(acct.Tags),
				"Provider": "azure",
			}
			if acct.Kind != nil {
				props["Kind"] = string(*acct.Kind)
			}
			if acct.SKU != nil && acct.SKU.Name != nil {
				props["SKU"] = string(*acct.SKU.Name)
			}

			// Storage cost depends on usage volume; nominal placeholder
			addNodeWithCost(s.graph, acctCRN, props, 5.0)
		}
	}
	return nil
}

// extractResourceGroup parses a resource group name from an Azure resource ID.
// Format: /subscriptions/{subId}/resourceGroups/{rg}/providers/{type}/{name}
func extractResourceGroup(id string) string {
	const marker = "/resourceGroups/"
	idx := 0
	for i, c := range id {
		if c == '/' && i > 0 {
			segment := id[idx:i]
			if segment == "resourceGroups" {
				start := i + 1
				for j := start; j < len(id); j++ {
					if id[j] == '/' {
						return id[start:j]
					}
				}
				return id[start:]
			}
		}
		if c == '/' {
			idx = i + 1
		}
	}
	_ = marker // suppress unused warning
	return ""
}
