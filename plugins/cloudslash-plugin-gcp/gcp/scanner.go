// SPDX-License-Identifier: Apache-2.0
// Copyright 2024-2026 CloudSlash Contributors

package gcp

import (
	"context"
	"fmt"

	"google.golang.org/api/bigquery/v2"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/container/v1"
	"google.golang.org/api/option"
	"google.golang.org/api/sqladmin/v1beta4"
	"google.golang.org/api/storage/v1"

	graph "github.com/DrSkyle/cloudslash-plugin-sdk/shared/scangraph"
)

// GCP OAuth2 scopes used for read-only access to GCP APIs.
const (
	scopeCloudPlatform    = "https://www.googleapis.com/auth/cloud-platform.read-only"
	scopeBigQueryReadOnly = "https://www.googleapis.com/auth/bigquery.readonly"
	scopeStorageReadOnly  = "https://www.googleapis.com/auth/devstorage.read_only"
)

// Scanner holds GCP API clients and the local graph for scan results.
type Scanner struct {
	projectID string
	graph     *graph.Graph
}

// NewScanner creates a GCP scanner using Application Default Credentials (ADC).
// Credentials resolve in order: GOOGLE_APPLICATION_CREDENTIALS env → gcloud auth → metadata server.
func NewScanner(projectID string, g *graph.Graph) *Scanner {
	return &Scanner{projectID: projectID, graph: g}
}

// ScanInstances scans all GCE VM instances across all zones.
func (s *Scanner) ScanInstances(ctx context.Context) error {
	svc, err := compute.NewService(ctx, option.WithScopes(scopeCloudPlatform))
	if err != nil {
		return fmt.Errorf("gcp: compute service: %w", err)
	}

	err = svc.Instances.AggregatedList(s.projectID).Pages(ctx, func(page *compute.InstanceAggregatedList) error {
		for zone, items := range page.Items {
			for _, inst := range items.Instances {
				id := fmt.Sprintf("%d", inst.Id)
				region := zoneToRegion(zone)
				instCRN := toCRN(region, "compute", "instance", id)

				machineShort := machineTypeShort(inst.MachineType)
				monthlyCost := gceCostPerMonth(machineShort)

				props := map[string]interface{}{
					"Name":         inst.Name,
					"Zone":         zone,
					"Status":       inst.Status,
					"MachineType":  machineShort,
					"CreationTime": inst.CreationTimestamp,
					"Labels":       inst.Labels,
					"Provider":     "gcp",
				}

				// Flag TERMINATED instances as waste candidates (still billed for persistent disks)
				if inst.Status == "TERMINATED" {
					props["IsWasteCandidate"] = true
					props["WasteReason"] = "GCE instance is TERMINATED — still incurring persistent disk costs"
					props["EstimatedMonthlyCost"] = monthlyCost
					addNodeWithCost(s.graph, instCRN, props, monthlyCost)
				} else {
					addNodeWithCost(s.graph, instCRN, props, monthlyCost)
				}
			}
		}
		return nil
	})
	return err
}

// ScanDisks scans all persistent disks and flags unattached ones.
func (s *Scanner) ScanDisks(ctx context.Context) error {
	svc, err := compute.NewService(ctx, option.WithScopes(scopeCloudPlatform))
	if err != nil {
		return fmt.Errorf("gcp: compute service: %w", err)
	}

	err = svc.Disks.AggregatedList(s.projectID).Pages(ctx, func(page *compute.DiskAggregatedList) error {
		for zone, items := range page.Items {
			for _, disk := range items.Disks {
				id := fmt.Sprintf("%d", disk.Id)
				region := zoneToRegion(zone)
				diskCRN := toCRN(region, "compute", "disk", id)

				diskType := diskTypeShort(disk.Type)
				monthlyCost := gcpDiskCostPerMonth(disk.SizeGb, diskType)

				props := map[string]interface{}{
					"Name":         disk.Name,
					"Zone":         zone,
					"Status":       disk.Status,
					"SizeGB":       disk.SizeGb,
					"DiskType":     diskType,
					"CreationTime": disk.CreationTimestamp,
					"Labels":       disk.Labels,
					"Provider":     "gcp",
				}

				// Flag disks with no users (unattached) as waste candidates
				if len(disk.Users) == 0 {
					props["IsWasteCandidate"] = true
					props["WasteReason"] = "persistent disk has no users — unattached and incurring idle storage cost"
					props["EstimatedMonthlyCost"] = monthlyCost
				}

				addNodeWithCost(s.graph, diskCRN, props, monthlyCost)
			}
		}
		return nil
	})
	return err
}

// ScanGKEClusters scans all GKE Kubernetes clusters.
func (s *Scanner) ScanGKEClusters(ctx context.Context) error {
	svc, err := container.NewService(ctx, option.WithScopes(scopeCloudPlatform))
	if err != nil {
		return fmt.Errorf("gcp: container service: %w", err)
	}

	// List clusters in all zones/regions using the wildcard parent.
	parent := fmt.Sprintf("projects/%s/locations/-", s.projectID)
	resp, err := svc.Projects.Locations.Clusters.List(parent).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("gcp: list GKE clusters: %w", err)
	}

	for _, cluster := range resp.Clusters {
		region := cluster.Location
		clusterCRN := toCRN(region, "container", "cluster", cluster.Name)

		autopilot := cluster.Autopilot != nil && cluster.Autopilot.Enabled
		monthlyCost := gkeCostPerMonth(cluster.CurrentNodeCount, autopilot)

		props := map[string]interface{}{
			"Name":           cluster.Name,
			"Location":       cluster.Location,
			"Status":         cluster.Status,
			"CurrentVersion": cluster.CurrentMasterVersion,
			"NodeCount":      cluster.CurrentNodeCount,
			"Autopilot":      autopilot,
			"CreateTime":     cluster.CreateTime,
			"Labels":         cluster.ResourceLabels,
			"Provider":       "gcp",
		}

		// Flag clusters in ERROR state as waste candidates
		if cluster.Status == "ERROR" || cluster.Status == "DEGRADED" {
			props["IsWasteCandidate"] = true
			props["WasteReason"] = fmt.Sprintf("GKE cluster is in %s state — incurring costs while unhealthy", cluster.Status)
			props["EstimatedMonthlyCost"] = monthlyCost
		}

		addNodeWithCost(s.graph, clusterCRN, props, monthlyCost)
	}
	return nil
}

// ScanCloudSQLInstances scans all Cloud SQL instances.
func (s *Scanner) ScanCloudSQLInstances(ctx context.Context) error {
	svc, err := sqladmin.NewService(ctx, option.WithScopes(scopeCloudPlatform))
	if err != nil {
		return fmt.Errorf("gcp: sql admin service: %w", err)
	}

	resp, err := svc.Instances.List(s.projectID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("gcp: list Cloud SQL instances: %w", err)
	}

	for _, inst := range resp.Items {
		region := inst.Region
		sqlCRN := toCRN(region, "sqladmin", "instance", inst.Name)

		tier := ""
		if inst.Settings != nil {
			tier = inst.Settings.Tier
		}
		monthlyCost := gcloudSQLCostPerMonth(tier)

		props := map[string]interface{}{
			"Name":            inst.Name,
			"Region":          region,
			"DatabaseVersion": inst.DatabaseVersion,
			"State":           inst.State,
			"Provider":        "gcp",
		}
		if inst.Settings != nil {
			props["Tier"] = inst.Settings.Tier
			if inst.Settings.BackupConfiguration != nil {
				props["BackupEnabled"] = inst.Settings.BackupConfiguration.Enabled
			}
			props["Labels"] = inst.Settings.UserLabels
		}

		// Flag STOPPED instances as waste candidates (still billing for storage)
		if inst.State == "SUSPENDED" || inst.State == "STOPPED" {
			props["IsWasteCandidate"] = true
			props["WasteReason"] = fmt.Sprintf("Cloud SQL instance is %s — still incurring storage costs", inst.State)
			props["EstimatedMonthlyCost"] = monthlyCost
		}

		addNodeWithCost(s.graph, sqlCRN, props, monthlyCost)
	}
	return nil
}

// ScanGCSBuckets scans all Cloud Storage buckets.
func (s *Scanner) ScanGCSBuckets(ctx context.Context) error {
	svc, err := storage.NewService(ctx, option.WithScopes(scopeStorageReadOnly))
	if err != nil {
		return fmt.Errorf("gcp: storage service: %w", err)
	}

	resp, err := svc.Buckets.List(s.projectID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("gcp: list GCS buckets: %w", err)
	}

	for _, bucket := range resp.Items {
		region := bucket.Location
		bucketCRN := toCRN(region, "storage", "bucket", bucket.Name)

		props := map[string]interface{}{
			"Name":               bucket.Name,
			"Location":           region,
			"StorageClass":       bucket.StorageClass,
			"Created":            bucket.TimeCreated,
			"Labels":             bucket.Labels,
			"VersioningEnabled":  bucket.Versioning != nil && bucket.Versioning.Enabled,
			"Provider":           "gcp",
		}

		// GCS cost depends on usage; set a minimal placeholder for non-empty heuristic
		addNodeWithCost(s.graph, bucketCRN, props, 0.0)
	}
	return nil
}

// ScanBigQueryDatasets scans all BigQuery datasets and their tables.
func (s *Scanner) ScanBigQueryDatasets(ctx context.Context) error {
	svc, err := bigquery.NewService(ctx, option.WithScopes(scopeBigQueryReadOnly))
	if err != nil {
		return fmt.Errorf("gcp: bigquery service: %w", err)
	}

	err = svc.Datasets.List(s.projectID).Pages(ctx, func(page *bigquery.DatasetList) error {
		for _, ds := range page.Datasets {
			dsID := ds.DatasetReference.DatasetId
			dsCRN := toCRN("us", "bigquery", "dataset", dsID)

			props := map[string]interface{}{
				"DatasetID": dsID,
				"Labels":    ds.Labels,
				"Location":  ds.Location,
				"Provider":  "gcp",
			}
			addNodeWithCost(s.graph, dsCRN, props, 0.0)
		}
		return nil
	})
	return err
}

// zoneToRegion strips the trailing zone letter: "us-central1-a" → "us-central1".
func zoneToRegion(zone string) string {
	for i := len(zone) - 1; i >= 0; i-- {
		if zone[i] == '-' {
			return zone[:i]
		}
	}
	return zone
}

// machineTypeShort extracts the type name from a full GCP URL.
func machineTypeShort(url string) string {
	for i := len(url) - 1; i >= 0; i-- {
		if url[i] == '/' {
			return url[i+1:]
		}
	}
	return url
}

// diskTypeShort extracts the disk type name from a full GCP URL.
func diskTypeShort(url string) string {
	return machineTypeShort(url)
}
