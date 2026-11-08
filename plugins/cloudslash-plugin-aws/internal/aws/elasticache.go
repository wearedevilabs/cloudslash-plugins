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
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
)

// ElasticacheScanner scans ElastiCache clusters.
type ElasticacheScanner struct {
	Client   *elasticache.Client
	CWClient *cloudwatch.Client
	Graph    *Collector
	Region   string
	Logger   *slog.Logger
}

// NewElasticacheScanner initializes a scanner for ElastiCache resources.
func NewElasticacheScanner(cfg aws.Config, g *Collector, logger *slog.Logger) *ElasticacheScanner {
	if logger == nil {
		logger = slog.Default()
	}
	return &ElasticacheScanner{
		Client:   elasticache.NewFromConfig(cfg),
		CWClient: cloudwatch.NewFromConfig(cfg),
		Graph:    g,
		Region:   cfg.Region,
		Logger:   logger,
	}
}

// ScanClusters scans clusters and analyzes metrics.
func (s *ElasticacheScanner) ScanClusters(ctx context.Context) error {
	paginator := elasticache.NewDescribeCacheClustersPaginator(s.Client, &elasticache.DescribeCacheClustersInput{
		ShowCacheNodeInfo: aws.Bool(true),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}

		for _, cluster := range page.CacheClusters {


			// Group by ClusterID.
			id := *cluster.CacheClusterId

			props := map[string]interface{}{
				"Service":       "Elasticache",
				"Engine":        *cluster.Engine,
				"Status":        *cluster.CacheClusterStatus,
				"NodeType":      *cluster.CacheNodeType,
				"EngineVersion": *cluster.EngineVersion,
				"NumCacheNodes": cluster.NumCacheNodes,
			}
			
			clusterCRN := toCRN(s.Region, "current", "elasticache", "cluster", id)
			s.Graph.AddNode(clusterCRN, props)

			// Fetch metrics.

			go s.enrichClusterMetrics(ctx, id, clusterCRN, cluster.CacheNodeType, props)
		}
	}
	return nil
}

// enrichClusterMetrics retrieves performance metrics.
func (s *ElasticacheScanner) enrichClusterMetrics(ctx context.Context, clusterID string, clusterCRN string, nodeType *string, props map[string]interface{}) {
	node := s.Graph.GetNode(clusterCRN)
	exists := (node != nil)
	if !exists {
		return
	}

	endTime := time.Now()
	startTime := endTime.Add(-7 * 24 * time.Hour) // 7-day window.

	// Metrics.
	queries := []cwtypes.MetricDataQuery{
		{
			Id: aws.String("m_conn"),
			MetricStat: &cwtypes.MetricStat{
				Metric: &cwtypes.Metric{
					Namespace:  aws.String("AWS/ElastiCache"),
					MetricName: aws.String("CurrConnections"),
					Dimensions: []cwtypes.Dimension{{Name: aws.String("CacheClusterId"), Value: aws.String(clusterID)}},
				},
				Period: aws.Int32(86400), // 1-day granularity.
				Stat:   aws.String("Sum"),
			},
		},
		{
			Id: aws.String("m_hits"),
			MetricStat: &cwtypes.MetricStat{
				Metric: &cwtypes.Metric{
					Namespace:  aws.String("AWS/ElastiCache"),
					MetricName: aws.String("CacheHits"),
					Dimensions: []cwtypes.Dimension{{Name: aws.String("CacheClusterId"), Value: aws.String(clusterID)}},
				},
				Period: aws.Int32(86400),
				Stat:   aws.String("Sum"),
			},
		},
		{
			Id: aws.String("m_misses"),
			MetricStat: &cwtypes.MetricStat{
				Metric: &cwtypes.Metric{
					Namespace:  aws.String("AWS/ElastiCache"),
					MetricName: aws.String("CacheMisses"),
					Dimensions: []cwtypes.Dimension{{Name: aws.String("CacheClusterId"), Value: aws.String(clusterID)}},
				},
				Period: aws.Int32(86400),
				Stat:   aws.String("Sum"),
			},
		},
		{
			Id: aws.String("m_cpu"),
			MetricStat: &cwtypes.MetricStat{
				Metric: &cwtypes.Metric{
					Namespace:  aws.String("AWS/ElastiCache"),
					MetricName: aws.String("CPUUtilization"),
					Dimensions: []cwtypes.Dimension{{Name: aws.String("CacheClusterId"), Value: aws.String(clusterID)}},
				},
				Period: aws.Int32(86400),
				Stat:   aws.String("Maximum"),
			},
		},
		{
			Id: aws.String("m_net"),
			MetricStat: &cwtypes.MetricStat{
				Metric: &cwtypes.Metric{
					Namespace:  aws.String("AWS/ElastiCache"),
					MetricName: aws.String("NetworkBytesIn"),
					Dimensions: []cwtypes.Dimension{{Name: aws.String("CacheClusterId"), Value: aws.String(clusterID)}},
				},
				Period: aws.Int32(86400),
				Stat:   aws.String("Sum"),
			},
		},
	}

	out, err := s.CWClient.GetMetricData(ctx, &cloudwatch.GetMetricDataInput{
		MetricDataQueries: queries,
		StartTime:         &startTime,
		EndTime:           &endTime,
	})

	if err != nil {
		s.Logger.Error("Error fetching ElastiCache metrics", "cluster", clusterID, "error", err)
		return
	}

	// Parse metrics.
	var totalConn, totalHits, totalMisses, totalNet float64
	var maxCPU float64

	for _, res := range out.MetricDataResults {
		sum := 0.0
		max := 0.0
		for _, val := range res.Values {
			sum += val
			if val > max {
				max = val
			}
		}

		switch *res.Id {
		case "m_conn":
			totalConn = sum
		case "m_hits":
			totalHits = sum
		case "m_misses":
			totalMisses = sum
		case "m_cpu":
			maxCPU = max // Peak CPU.
		case "m_net":
			totalNet = sum
		}
	}

	// Update node.
	node.Properties["SumConnections7d"] = totalConn
	node.Properties["SumHits7d"] = totalHits
	node.Properties["SumMisses7d"] = totalMisses
	node.Properties["MaxCPU7d"] = maxCPU
	node.Properties["SumNetworkBytesIn7d"] = totalNet
}
