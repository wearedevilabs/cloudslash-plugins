// SPDX-License-Identifier: Apache-2.0
// Copyright 2024-2026 CloudSlash Contributors

package aws

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

// Enrich attaches provider-specific metrics to the collected nodes as generic
// properties so the core's CEL rules can evaluate them. It never marks waste or
// applies policy — that is the engine's job. Each enricher is fail-open: if the
// underlying cloud API is unavailable the property is simply omitted.
func Enrich(ctx context.Context, cfg aws.Config, c *Collector) {
	enrichEC2CPUUtilization(ctx, cfg, c)
}

// enrichEC2CPUUtilization records the maximum CPU utilization over the last 7
// days for each running EC2 instance as props["metrics_cpu_max_7d"]. Core CEL
// rules use it for idle-instance detection (e.g. max CPU < 2%).
func enrichEC2CPUUtilization(ctx context.Context, cfg aws.Config, c *Collector) {
	end := time.Now()
	start := end.Add(-7 * 24 * time.Hour)

	for _, n := range c.nodes {
		region, service, typ, _, id := parseCRN(n.CRN)
		if service != "ec2" || typ != "instance" || id == "" {
			continue
		}
		if n.Properties["State"] != "running" {
			continue
		}

		cfg := cfg.Copy()
		cfg.Region = region
		out, err := cloudwatch.NewFromConfig(cfg).GetMetricStatistics(ctx, &cloudwatch.GetMetricStatisticsInput{
			Namespace:  aws.String("AWS/EC2"),
			MetricName: aws.String("CPUUtilization"),
			Dimensions: []cwtypes.Dimension{{Name: aws.String("InstanceId"), Value: aws.String(id)}},
			StartTime:  aws.Time(start),
			EndTime:    aws.Time(end),
			Period:     aws.Int32(3600),
			Statistics: []cwtypes.Statistic{cwtypes.StatisticMaximum},
		})
		if err != nil || len(out.Datapoints) == 0 {
			continue
		}
		var max float64
		for _, dp := range out.Datapoints {
			if dp.Maximum != nil && *dp.Maximum > max {
				max = *dp.Maximum
			}
		}
		n.Properties["metrics_cpu_max_7d"] = max
	}
}
