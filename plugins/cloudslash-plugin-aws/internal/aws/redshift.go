// SPDX-License-Identifier: Apache-2.0
// Copyright 2024-2026 CloudSlash Contributors

package aws

import (
	"context"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/redshift"
)

type RedshiftScanner struct {
	Client   *redshift.Client
	CWClient *cloudwatch.Client
	Graph    *Collector
	Region   string
	Logger   *slog.Logger
}

func NewRedshiftScanner(cfg aws.Config, g *Collector, logger *slog.Logger) *RedshiftScanner {
	if logger == nil {
		logger = slog.Default()
	}
	return &RedshiftScanner{
		Client:   redshift.NewFromConfig(cfg),
		CWClient: cloudwatch.NewFromConfig(cfg),
		Graph:    g,
		Region:   cfg.Region,
		Logger:   logger,
	}
}

// ScanClusters scans Redshift clusters.
func (s *RedshiftScanner) ScanClusters(ctx context.Context) error {
	paginator := redshift.NewDescribeClustersPaginator(s.Client, &redshift.DescribeClustersInput{})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}

		for _, cluster := range page.Clusters {
			id := *cluster.ClusterIdentifier

			props := map[string]interface{}{
				"Service":                   "Redshift",
				"NodeType":                  *cluster.NodeType,
				"ClusterStatus":             *cluster.ClusterStatus, // e.g., "available"
				"ClusterAvailabilityStatus": *cluster.ClusterAvailabilityStatus,
				"NumberOfNodes":             cluster.NumberOfNodes,
			}

			// Add to Graph
			clusterCRN := toCRN(s.Region, "current", "redshift", "cluster", id)
			s.Graph.AddNode(clusterCRN, props)

			// Add metrics.
			go s.enrichClusterMetrics(ctx, id, clusterCRN, props)

			// TODO: Check RI.
			// Assume On-Demand.
		}
	}
	return nil
}

// enrichClusterMetrics retrieves usage metrics.
func (s *RedshiftScanner) enrichClusterMetrics(ctx context.Context, clusterID string, clusterCRN string, props map[string]interface{}) {
	node := s.Graph.GetNode(clusterCRN)
	exists := (node != nil)
	if !exists {
		return
	}

	endTime := time.Now()
	startTime := endTime.Add(-24 * time.Hour) // 24-hour window.

	queries := []cwtypes.MetricDataQuery{
		{
			Id: aws.String("m_queries"),
			MetricStat: &cwtypes.MetricStat{
				Metric: &cwtypes.Metric{
					Namespace:  aws.String("AWS/Redshift"),
					MetricName: aws.String("QueriesCompletedPerSecond"),
					Dimensions: []cwtypes.Dimension{{Name: aws.String("ClusterIdentifier"), Value: aws.String(clusterID)}},
				},
				Period: aws.Int32(3600), // 1-hour granularity.
				Stat:   aws.String("Sum"),
			},
		},
		{
			Id: aws.String("m_conns"),
			MetricStat: &cwtypes.MetricStat{
				Metric: &cwtypes.Metric{
					Namespace:  aws.String("AWS/Redshift"),
					MetricName: aws.String("DatabaseConnections"),
					Dimensions: []cwtypes.Dimension{{Name: aws.String("ClusterIdentifier"), Value: aws.String(clusterID)}},
				},
				Period: aws.Int32(3600),
				Stat:   aws.String("Maximum"),
			},
		},
	}

	out, err := s.CWClient.GetMetricData(ctx, &cloudwatch.GetMetricDataInput{
		MetricDataQueries: queries,
		StartTime:         &startTime,
		EndTime:           &endTime,
	})

	if err != nil {
		s.Logger.Error("Error fetching Redshift metrics", "cluster", clusterID, "error", err)
		return
	}

	var totalQueries, maxConns float64
	for _, res := range out.MetricDataResults {
		sum := 0.0
		max := 0.0
		for _, v := range res.Values {
			sum += v
			if v > max {
				max = v
			}
		}
		if *res.Id == "m_queries" {
			totalQueries = sum
		} else if *res.Id == "m_conns" {
			maxConns = max
		}
	}

	node.Properties["SumQueries24h"] = totalQueries
	node.Properties["MaxConnections24h"] = maxConns
}
