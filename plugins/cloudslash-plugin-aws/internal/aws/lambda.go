// SPDX-License-Identifier: Apache-2.0
// Copyright 2024-2026 CloudSlash Contributors

package aws

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
)

// LambdaScanner scans Lambda functions.
type LambdaScanner struct {
	Client   *lambda.Client
	CWClient *cloudwatch.Client
	Graph    *Collector
	Region   string
}

func NewLambdaScanner(cfg aws.Config, g *Collector) *LambdaScanner {
	return &LambdaScanner{
		Client:   lambda.NewFromConfig(cfg),
		CWClient: cloudwatch.NewFromConfig(cfg),
		Graph:    g,
		Region:   cfg.Region,
	}
}

// ScanFunctions scans functions and checks usage.
func (s *LambdaScanner) ScanFunctions(ctx context.Context) error {
	paginator := lambda.NewListFunctionsPaginator(s.Client, &lambda.ListFunctionsInput{})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}

		for _, fn := range page.Functions {
			name := *fn.FunctionName
			arn := *fn.FunctionArn

			props := map[string]interface{}{
				"Service":      "Lambda",
				"Runtime":      string(fn.Runtime),
				"LastModified": *fn.LastModified, // Timestamp string.
				"CodeSize":     fn.CodeSize,
				"MemorySize":   fn.MemorySize,
			}

			lambdaCRN := toCRN(s.Region, "current", "lambda", "function", name)
			s.Graph.AddNode(lambdaCRN, props)

			// Analyze execution metrics to detect potential staleness (code rot).
			go s.checkCodeRot(ctx, name, props)

			// Catalog function versions and aliases.
			go s.scanVersionsAndAliases(ctx, name, arn)
		}
	}
	return nil
}

// checkCodeRot checks invocations over 90 days.
func (s *LambdaScanner) checkCodeRot(ctx context.Context, funcName string, props map[string]interface{}) {
	node := s.Graph.GetNode(toCRN(s.Region, "current", "lambda", "function", funcName))
	exists := (node != nil)
	if !exists {
		return
	}

	endTime := time.Now()
	startTime := endTime.Add(-90 * 24 * time.Hour) // 90-day window.

	queries := []cwtypes.MetricDataQuery{
		{
			Id: aws.String("m_invocations"),
			MetricStat: &cwtypes.MetricStat{
				Metric: &cwtypes.Metric{
					Namespace:  aws.String("AWS/Lambda"),
					MetricName: aws.String("Invocations"),
					Dimensions: []cwtypes.Dimension{{Name: aws.String("FunctionName"), Value: aws.String(funcName)}},
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
		return
	}

	totalInvocations := 0.0
	for _, res := range out.MetricDataResults {
		for _, v := range res.Values {
			totalInvocations += v
		}
	}

	node.Properties["SumInvocations90d"] = totalInvocations
}

// scanVersionsAndAliases maps versions and aliases.
func (s *LambdaScanner) scanVersionsAndAliases(ctx context.Context, funcName string, funcArn string) {
	// Get aliases.
	aliases := make(map[string]bool) // Key: Version ID

	aPaginator := lambda.NewListAliasesPaginator(s.Client, &lambda.ListAliasesInput{FunctionName: aws.String(funcName)})
	for aPaginator.HasMorePages() {
		page, err := aPaginator.NextPage(ctx)
		if err == nil {
			for _, alias := range page.Aliases {
				aliases[*alias.FunctionVersion] = true
			}
		}
	}

	// Get versions.
	vPaginator := lambda.NewListVersionsByFunctionPaginator(s.Client, &lambda.ListVersionsByFunctionInput{FunctionName: aws.String(funcName)})

	var versions []string
	var totalSize int64

	for vPaginator.HasMorePages() {
		page, err := vPaginator.NextPage(ctx)
		if err != nil {
			break
		}

		for _, v := range page.Versions {
			if *v.Version == "$LATEST" {
				continue // Exclude $LATEST.
			}
			versions = append(versions, *v.Version)
			totalSize += v.CodeSize
		}
	}

	node := s.Graph.GetNode(toCRN(s.Region, "current", "lambda", "function", funcName))
	if node != nil {
		node.Properties["AllVersions"] = versions
		node.Properties["AliasVersions"] = aliases // map[string]bool
		node.Properties["TotalCodeSizeBytes"] = totalSize
		node.Properties["VersionCount"] = len(versions)
	}
}
