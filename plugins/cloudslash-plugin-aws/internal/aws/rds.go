// SPDX-License-Identifier: Apache-2.0
// Copyright 2024-2026 CloudSlash Contributors

package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
)

// RDSScanner scans RDS instances.
type RDSScanner struct {
	Client *rds.Client
	Graph  *Collector
	Region string
}

// NewRDSScanner initializes a scanner for RDS resources.
func NewRDSScanner(cfg aws.Config, g *Collector) *RDSScanner {
	return &RDSScanner{
		Client: rds.NewFromConfig(cfg),
		Graph:  g,
		Region: cfg.Region,
	}
}

// ScanInstances maps RDS instances.
func (s *RDSScanner) ScanInstances(ctx context.Context) error {
	paginator := rds.NewDescribeDBInstancesPaginator(s.Client, &rds.DescribeDBInstancesInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to describe rds instances: %v", err)
		}

		for _, instance := range page.DBInstances {

			id := *instance.DBInstanceIdentifier
			rdsCRN := toCRN(s.Region, "current", "rds", "db-instance", id)

			props := map[string]interface{}{
				"Status":        *instance.DBInstanceStatus,
				"InstanceClass": *instance.DBInstanceClass,
				"Engine":        *instance.Engine,
			}

			s.Graph.AddNode(rdsCRN, props)
		}
	}
	return nil
}
