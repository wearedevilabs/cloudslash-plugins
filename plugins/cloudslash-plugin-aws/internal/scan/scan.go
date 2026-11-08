// SPDX-License-Identifier: Apache-2.0
// Copyright 2024-2026 CloudSlash Contributors

// Package scan orchestrates the AWS provider scan: it connects to AWS, collects
// resources into normalized nodes/edges, attaches provider metrics, and streams
// the result back to the engine. It deliberately performs no heuristic or policy
// evaluation — the engine owns that.
package scan

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	aws "github.com/DrSkyle/cloudslash-plugin-aws/internal/aws"
	pb "github.com/DrSkyle/cloudslash-plugin-sdk/shared/proto"
)

// Run executes a full scan for the given config and streams nodes and edges to
// the engine. It returns the first non-nil scanner error; partial results are
// still streamed.
func Run(ctx context.Context, config map[string]string, stream pb.Provider_ScanServer) error {
	region := config["region"]
	profile := config["profile"]

	logger := slog.Default().With("component", "aws-plugin")

	client, err := aws.NewClient(ctx, region, profile, false, logger)
	if err != nil {
		return fmt.Errorf("aws plugin: client init: %w", err)
	}

	c := aws.NewCollector()

	ec2Scanner := aws.NewEC2Scanner(client.Config, c)
	s3Scanner := aws.NewS3Scanner(client.Config, c, logger)
	rdsScanner := aws.NewRDSScanner(client.Config, c)
	eksScanner := aws.NewEKSScanner(client.Config, c, logger)
	natScanner := aws.NewNATScanner(client.Config, c)
	eipScanner := aws.NewEIPScanner(client.Config, c)
	albScanner := aws.NewALBScanner(client.Config, c)
	vpcepScanner := aws.NewVpcEndpointScanner(client.Config, c)
	ecsScanner := aws.NewECSScanner(client.Config, c, logger)
	elasticacheScanner := aws.NewElasticacheScanner(client.Config, c, logger)
	redshiftScanner := aws.NewRedshiftScanner(client.Config, c, logger)
	dynamoScanner := aws.NewDynamoDBScanner(client.Config, c)
	lambdaScanner := aws.NewLambdaScanner(client.Config, c)

	scanners := []struct {
		name string
		fn   func(context.Context) error
	}{
		{"EC2Instances", ec2Scanner.ScanInstances},
		{"EC2Volumes", ec2Scanner.ScanVolumes},
		{"EC2Snapshots", func(ctx context.Context) error { return ec2Scanner.ScanSnapshots(ctx, "self") }},
		{"EC2Images", ec2Scanner.ScanImages},
		{"NATGateways", natScanner.ScanNATGateways},
		{"ElasticIPs", eipScanner.ScanAddresses},
		{"ALBs", albScanner.ScanALBs},
		{"VpcEndpoints", vpcepScanner.ScanEndpoints},
		{"S3Buckets", s3Scanner.ScanBuckets},
		{"RDSInstances", rdsScanner.ScanInstances},
		{"EKSClusters", eksScanner.ScanClusters},
		{"ECSClusters", ecsScanner.ScanClusters},
		{"ElasticacheClusters", elasticacheScanner.ScanClusters},
		{"RedshiftClusters", redshiftScanner.ScanClusters},
		{"DynamoDBTables", dynamoScanner.ScanTables},
		{"LambdaFunctions", lambdaScanner.ScanFunctions},
	}

	var scanErrors []error
	for _, sc := range scanners {
		if err := sc.fn(ctx); err != nil {
			logger.Warn("Scanner warning", "scanner", sc.name, "error", err)
			scanErrors = append(scanErrors, fmt.Errorf("%s: %w", sc.name, err))
		}
	}

	// Attach provider-specific metrics as generic node properties so the core's
	// CEL rules can evaluate them.
	aws.Enrich(ctx, client.Config, c)

	// Stream nodes in batches of 50.
	const batchSize = 50
	nodeBatch := c.Nodes()
	for i := 0; i < len(nodeBatch); i += batchSize {
		end := i + batchSize
		if end > len(nodeBatch) {
			end = len(nodeBatch)
		}
		if err := stream.Send(&pb.ScanResponse{Nodes: nodeBatch[i:end]}); err != nil {
			return err
		}
	}

	// Stream edges in batches of 50.
	edgeBatch := c.Edges()
	for i := 0; i < len(edgeBatch); i += batchSize {
		end := i + batchSize
		if end > len(edgeBatch) {
			end = len(edgeBatch)
		}
		if err := stream.Send(&pb.ScanResponse{Edges: edgeBatch[i:end]}); err != nil {
			return err
		}
	}

	return errors.Join(scanErrors...)
}
