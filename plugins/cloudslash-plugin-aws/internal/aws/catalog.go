// SPDX-License-Identifier: Apache-2.0
// Copyright 2024-2026 CloudSlash Contributors

package aws

import (
	"github.com/DrSkyle/cloudslash-plugin-sdk/shared/permissions"
)

func init() {
	permissions.RegisterCatalog("aws", &permissions.Catalog{
		Sets: []permissions.PermissionSet{
			{
				ResourceFamily: "compute",
				ReadActions: []string{
					"ec2:DescribeInstances",
					"ec2:DescribeVolumes",
					"ec2:DescribeNatGateways",
					"ec2:DescribeAddresses",
					"ec2:DescribeSnapshots",
					"ec2:DescribeImages",
					"ec2:DescribeVolumesModifications",
					"ec2:DescribeInstanceTypes",
					"ec2:DescribeSecurityGroups",
					"ec2:DescribeSubnets",
					"ec2:DescribeVpcs",
				},
			},
			{
				ResourceFamily: "object-storage",
				ReadActions: []string{
					"s3:ListAllMyBuckets",
					"s3:GetBucketLocation",
					"s3:GetBucketTagging",
					"s3:GetBucketVersioning",
					"s3:GetLifecycleConfiguration",
					"s3:ListBucket",
				},
			},
			{
				ResourceFamily: "identity",
				ReadActions: []string{
					"iam:ListUsers",
					"iam:ListRoles",
					"iam:GetAccessKeyLastUsed",
					"iam:ListAccessKeys",
					"iam:GetUser",
					"iam:GetRole",
				},
			},
			{
				ResourceFamily: "database",
				ReadActions: []string{
					"rds:DescribeDBInstances",
					"rds:DescribeDBClusters",
					"rds:ListTagsForResource",
				},
			},
			{
				ResourceFamily: "container",
				ReadActions: []string{
					"eks:ListClusters",
					"eks:DescribeCluster",
					"eks:ListNodegroups",
					"eks:DescribeNodegroup",
					"eks:ListFargateProfiles",
					"eks:DescribeFargateProfile",
				},
			},
			{
				ResourceFamily: "compute/function",
				ReadActions: []string{
					"lambda:ListFunctions",
					"lambda:GetFunction",
					"lambda:ListTags",
				},
			},
			{
				ResourceFamily: "observability",
				ReadActions: []string{
					"cloudwatch:GetMetricData",
					"cloudwatch:ListMetrics",
				},
			},
		},
	})
}
