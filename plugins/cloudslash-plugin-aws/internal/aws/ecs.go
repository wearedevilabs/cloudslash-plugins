// SPDX-License-Identifier: Apache-2.0
// Copyright 2024-2026 CloudSlash Contributors

package aws

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

// ECSScanner scans ECS clusters and services.
type ECSScanner struct {
	Client *ecs.Client
	Graph  *Collector
	Region string
	Logger *slog.Logger
}

func NewECSScanner(cfg aws.Config, g *Collector, logger *slog.Logger) *ECSScanner {
	if logger == nil {
		logger = slog.Default()
	}
	return &ECSScanner{
		Client: ecs.NewFromConfig(cfg),
		Graph:  g,
		Region: cfg.Region,
		Logger: logger,
	}
}

// ScanClusters scans clusters and their components.
func (s *ECSScanner) ScanClusters(ctx context.Context) error {
	paginator := ecs.NewListClustersPaginator(s.Client, &ecs.ListClustersInput{})
	var clusterARNs []string

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}
		clusterARNs = append(clusterARNs, page.ClusterArns...)
	}

	if len(clusterARNs) == 0 {
		return nil
	}

	// Process clusters in batches of 100 to respect API limits.
	chunkSize := 100
	for i := 0; i < len(clusterARNs); i += chunkSize {
		end := i + chunkSize
		if end > len(clusterARNs) {
			end = len(clusterARNs)
		}
		chunk := clusterARNs[i:end]

		output, err := s.Client.DescribeClusters(ctx, &ecs.DescribeClustersInput{
			Clusters: chunk,
			Include:  []types.ClusterField{types.ClusterFieldTags},
		})
		if err != nil {
			s.Logger.Error("Error describing ECS clusters", "error", err)
			continue
		}

		for _, cluster := range output.Clusters {
			s.addClusterNode(cluster)
			// Recursively scan cluster components.
			if err := s.ScanServices(ctx, *cluster.ClusterName); err != nil {
				s.Logger.Error("Error scanning ECS services for cluster", "cluster", *cluster.ClusterName, "error", err)
			}
			if err := s.ScanContainerInstances(ctx, *cluster.ClusterName); err != nil {
				s.Logger.Error("Error scanning ECS container instances for cluster", "cluster", *cluster.ClusterName, "error", err)
			}
		}
	}

	return nil
}

// addClusterNode adds a cluster node to the graph.
func (s *ECSScanner) addClusterNode(cluster types.Cluster) {
	clusterCRN := toCRN(s.Region, "current", "ecs", "cluster", *cluster.ClusterName)
	s.Graph.AddNode(clusterCRN, map[string]interface{}{
		"Name":                              *cluster.ClusterName,
		"Status":                            *cluster.Status,
		"RegisteredContainerInstancesCount": int(cluster.RegisteredContainerInstancesCount),
		"RunningTasksCount":                 int(cluster.RunningTasksCount),
		"PendingTasksCount":                 int(cluster.PendingTasksCount),
		"ActiveServicesCount":               int(cluster.ActiveServicesCount),
	})
}

// ScanServices scans services in a cluster.
func (s *ECSScanner) ScanServices(ctx context.Context, clusterName string) error {
	paginator := ecs.NewListServicesPaginator(s.Client, &ecs.ListServicesInput{
		Cluster: aws.String(clusterName),
	})

	var serviceARNs []string
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}
		serviceARNs = append(serviceARNs, page.ServiceArns...)
	}

	if len(serviceARNs) == 0 {
		return nil
	}

	chunkSize := 10
	for i := 0; i < len(serviceARNs); i += chunkSize {
		end := i + chunkSize
		if end > len(serviceARNs) {
			end = len(serviceARNs)
		}
		chunk := serviceARNs[i:end]

		output, err := s.Client.DescribeServices(ctx, &ecs.DescribeServicesInput{
			Cluster:  aws.String(clusterName),
			Services: chunk,
		})
		if err != nil {
			s.Logger.Error("Error describing ECS services", "error", err)
			continue
		}

		for _, service := range output.Services {
			s.addServiceNode(service, clusterName)
		}
	}
	return nil
}

// addServiceNode links a service to its cluster.
func (s *ECSScanner) addServiceNode(service types.Service, clusterName string) {
	events := []string{}
	// Capture events.
	for i := 0; i < len(service.Events) && i < 3; i++ {
		events = append(events, *service.Events[i].Message)
	}

	// Get TaskDef.
	taskDef := ""
	if service.TaskDefinition != nil {
		taskDef = *service.TaskDefinition
	}

	id := fmt.Sprintf("%s/%s", clusterName, *service.ServiceName)
	serviceCRN := toCRN(s.Region, "current", "ecs", "service", id)
	clusterCRN := toCRN(s.Region, "current", "ecs", "cluster", clusterName)

	s.Graph.AddNode(serviceCRN, map[string]interface{}{
		"Name":           *service.ServiceName,
		"ClusterName":    clusterName,
		"Status":         *service.Status,
		"DesiredCount":   int(service.DesiredCount),
		"RunningCount":   int(service.RunningCount),
		"PendingCount":   int(service.PendingCount),
		"LaunchType":     string(service.LaunchType),
		"TaskDefinition": taskDef,
		"Events":         events,
	})
	s.Graph.AddTypedEdge(clusterCRN, serviceCRN, EdgeTypeContains, 1)
}

// ScanContainerInstances scans container instances in a cluster.
func (s *ECSScanner) ScanContainerInstances(ctx context.Context, clusterName string) error {
	paginator := ecs.NewListContainerInstancesPaginator(s.Client, &ecs.ListContainerInstancesInput{
		Cluster: aws.String(clusterName),
	})

	var instanceARNs []string
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}
		instanceARNs = append(instanceARNs, page.ContainerInstanceArns...)
	}

	if len(instanceARNs) == 0 {
		return nil
	}

	chunkSize := 100
	for i := 0; i < len(instanceARNs); i += chunkSize {
		end := i + chunkSize
		if end > len(instanceARNs) {
			end = len(instanceARNs)
		}
		chunk := instanceARNs[i:end]

		output, err := s.Client.DescribeContainerInstances(ctx, &ecs.DescribeContainerInstancesInput{
			Cluster:            aws.String(clusterName),
			ContainerInstances: chunk,
		})
		if err != nil {
			continue
		}

		for _, ci := range output.ContainerInstances {
			// Add Node.
			// Map EC2 instance.
			ec2InstanceID := *ci.Ec2InstanceId

			ciCRN := toCRN(s.Region, "current", "ecs", "container-instance", *ci.ContainerInstanceArn)
			clusterCRN := toCRN(s.Region, "current", "ecs", "cluster", clusterName)
			ec2CRN := toCRN(s.Region, "current", "ec2", "instance", ec2InstanceID)

			s.Graph.AddNode(ciCRN, map[string]interface{}{
				"ClusterName":   clusterName,
				"Ec2InstanceId": ec2InstanceID,
				"RegisteredAt":  ci.RegisteredAt,
				"Status":        *ci.Status,
			})
			s.Graph.AddTypedEdge(clusterCRN, ciCRN, EdgeType("HAS_INSTANCE"), 1)

			// Create EC2 edge.
			s.Graph.AddTypedEdge(ciCRN, ec2CRN, EdgeType("RUNS_ON"), 1)
		}
	}
	return nil
}
