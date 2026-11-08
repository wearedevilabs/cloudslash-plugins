// SPDX-License-Identifier: Apache-2.0
// Copyright 2024-2026 CloudSlash Contributors

package aws

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	pb "github.com/DrSkyle/cloudslash-plugin-sdk/shared/proto"
)

// Remediate executes a single cloud mutation for the resource identified by a
// CRN. It returns only a generic status and an optional rollback snapshot;
// orchestration, approval, saga rollback and state tracking live in the
// engine's s.a.u layer. The core never calls cloud SDKs directly.
func Remediate(ctx context.Context, req *pb.RemediateRequest) (*pb.RemediateResponse, error) {
	if req == nil {
		return &pb.RemediateResponse{Success: false, Error: "missing remediation request"}, nil
	}
	region, service, typ, account, id := parseCRN(req.Crn)
	if id == "" || account == "" {
		return &pb.RemediateResponse{Success: false, Error: "invalid crn: " + req.Crn}, nil
	}
	if region == "" {
		region = req.Properties["region"]
	}
	if region == "" {
		return &pb.RemediateResponse{Success: false, Error: "resource region is required"}, nil
	}

	action := strings.ToLower(req.Action)
	if err := validateRemediationAction(service, typ, action); err != nil {
		return &pb.RemediateResponse{Success: false, Error: err.Error()}, nil
	}
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return &pb.RemediateResponse{Success: false, Error: err.Error()}, nil
	}

	identity, err := sts.NewFromConfig(cfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return &pb.RemediateResponse{Success: false, Error: "cannot verify caller account: " + err.Error()}, nil
	}
	if identity == nil || aws.ToString(identity.Account) != account {
		return &pb.RemediateResponse{Success: false, Error: "credential account does not match resource CRN"}, nil
	}
	switch service + "/" + typ {
	case "ec2/instance":
		return remediateEC2Instance(ctx, cfg, id, action, req.DryRun)
	case "ec2/volume":
		return remediateEBSVolume(ctx, cfg, id, action, req.DryRun)
	case "rds/db-instance":
		return remediateRDS(ctx, cfg, id, action, req.DryRun)
	default:
		return &pb.RemediateResponse{Success: false, Error: fmt.Sprintf("unsupported resource %s/%s", service, typ)}, nil
	}
}

func remediateEC2Instance(ctx context.Context, cfg aws.Config, id, action string, dryRun bool) (*pb.RemediateResponse, error) {
	if err := validateRemediationAction("ec2", "instance", action); err != nil {
		return &pb.RemediateResponse{Success: false, Error: err.Error()}, nil
	}
	client := ec2.NewFromConfig(cfg)

	// Validate termination protection on dry-run.
	out, err := client.DescribeInstanceAttribute(ctx, &ec2.DescribeInstanceAttributeInput{
		InstanceId: aws.String(id),
		Attribute:  ec2types.InstanceAttributeNameDisableApiTermination,
	})
	if err != nil {
		return &pb.RemediateResponse{Success: false, Error: err.Error()}, nil
	}
	if out.DisableApiTermination != nil && aws.ToBool(out.DisableApiTermination.Value) {
		return &pb.RemediateResponse{Success: false, Error: "termination protection enabled"}, nil
	}
	if dryRun {
		return &pb.RemediateResponse{Success: true, Status: "ready"}, nil
	}

	switch action {
	case "start":
		if _, err := client.StartInstances(ctx, &ec2.StartInstancesInput{InstanceIds: []string{id}}); err != nil {
			return &pb.RemediateResponse{Success: false, Error: err.Error()}, nil
		}
		return &pb.RemediateResponse{Success: true, Status: "running"}, nil
	default: // "stop", "terminate"
		if _, err := client.CreateTags(ctx, &ec2.CreateTagsInput{
			Resources: []string{id},
			Tags: []ec2types.Tag{
				{Key: aws.String("CloudSlash:Status"), Value: aws.String("Purgatory")},
				{Key: aws.String("CloudSlash:ExpiryDate"), Value: aws.String(time.Now().AddDate(0, 0, 30).Format("2006-01-02"))},
			},
		}); err != nil {
			return &pb.RemediateResponse{Success: false, Error: err.Error()}, nil
		}
		if action == "terminate" || action == "delete" {
			if _, err := client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{InstanceIds: []string{id}}); err != nil {
				return &pb.RemediateResponse{Success: false, Error: err.Error()}, nil
			}
			return &pb.RemediateResponse{Success: true, Status: "terminated", Snapshot: map[string]string{"instance_id": id}}, nil
		}
		if _, err := client.StopInstances(ctx, &ec2.StopInstancesInput{InstanceIds: []string{id}}); err != nil {
			return &pb.RemediateResponse{Success: false, Error: err.Error()}, nil
		}
		return &pb.RemediateResponse{Success: true, Status: "stopped", Snapshot: map[string]string{"instance_id": id}}, nil
	}
}

func remediateEBSVolume(ctx context.Context, cfg aws.Config, id, action string, dryRun bool) (*pb.RemediateResponse, error) {
	if err := validateRemediationAction("ec2", "volume", action); err != nil {
		return &pb.RemediateResponse{Success: false, Error: err.Error()}, nil
	}
	client := ec2.NewFromConfig(cfg)

	// Validate not in-use on dry-run.
	out, err := client.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{VolumeIds: []string{id}})
	if err != nil {
		return &pb.RemediateResponse{Success: false, Error: err.Error()}, nil
	}
	if len(out.Volumes) != 1 || out.Volumes[0].State != ec2types.VolumeStateAvailable {
		return &pb.RemediateResponse{Success: false, Error: "volume must exist and be available"}, nil
	}
	if dryRun {
		return &pb.RemediateResponse{Success: true, Status: "ready"}, nil
	}

	// Safety snapshot before delete.
	snap, err := client.CreateSnapshot(ctx, &ec2.CreateSnapshotInput{
		VolumeId:    aws.String(id),
		Description: aws.String(fmt.Sprintf("CloudSlash safety snapshot for %s", id)),
		TagSpecifications: []ec2types.TagSpecification{
			{
				ResourceType: ec2types.ResourceTypeSnapshot,
				Tags: []ec2types.Tag{
					{Key: aws.String("CreatedBy"), Value: aws.String("CloudSlash")},
					{Key: aws.String("SourceVolume"), Value: aws.String(id)},
				},
			},
		},
	})
	if err != nil {
		return &pb.RemediateResponse{Success: false, Error: err.Error()}, nil
	}

	if snap.SnapshotId == nil || *snap.SnapshotId == "" {
		return &pb.RemediateResponse{Success: false, Error: "safety snapshot ID missing; volume retained"}, nil
	}
	if err := ec2.NewSnapshotCompletedWaiter(client).Wait(ctx, &ec2.DescribeSnapshotsInput{SnapshotIds: []string{*snap.SnapshotId}}, 10*time.Minute); err != nil {
		return &pb.RemediateResponse{Success: false, Error: "safety snapshot did not complete; volume retained: " + err.Error()}, nil
	}
	if _, err := client.DeleteVolume(ctx, &ec2.DeleteVolumeInput{VolumeId: aws.String(id)}); err != nil {
		return &pb.RemediateResponse{Success: false, Error: err.Error()}, nil
	}

	snapshotID := ""
	if snap.SnapshotId != nil {
		snapshotID = *snap.SnapshotId
	}
	return &pb.RemediateResponse{Success: true, Status: "deleted", Snapshot: map[string]string{"snapshot_id": snapshotID}}, nil
}

func remediateRDS(ctx context.Context, cfg aws.Config, id, action string, dryRun bool) (*pb.RemediateResponse, error) {
	if err := validateRemediationAction("rds", "db-instance", action); err != nil {
		return &pb.RemediateResponse{Success: false, Error: err.Error()}, nil
	}
	client := rds.NewFromConfig(cfg)

	// Validate deletion protection on dry-run.
	out, err := client.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{DBInstanceIdentifier: aws.String(id)})
	if err != nil {
		return &pb.RemediateResponse{Success: false, Error: err.Error()}, nil
	}
	if len(out.DBInstances) != 1 {
		return &pb.RemediateResponse{Success: false, Error: "database instance not found"}, nil
	}
	if action == "delete" && aws.ToBool(out.DBInstances[0].DeletionProtection) {
		return &pb.RemediateResponse{Success: false, Error: "deletion protection enabled"}, nil
	}
	if dryRun {
		return &pb.RemediateResponse{Success: true, Status: "ready"}, nil
	}

	switch action {
	case "start":
		if _, err := client.StartDBInstance(ctx, &rds.StartDBInstanceInput{DBInstanceIdentifier: aws.String(id)}); err != nil {
			return &pb.RemediateResponse{Success: false, Error: err.Error()}, nil
		}
		return &pb.RemediateResponse{Success: true, Status: "running"}, nil
	default: // "stop", "delete"
		if action == "delete" {
			if _, err := client.DeleteDBInstance(ctx, &rds.DeleteDBInstanceInput{
				DBInstanceIdentifier:      aws.String(id),
				SkipFinalSnapshot:         aws.Bool(false),
				FinalDBSnapshotIdentifier: aws.String("cloudslash-final-" + id),
			}); err != nil {
				return &pb.RemediateResponse{Success: false, Error: err.Error()}, nil
			}
			return &pb.RemediateResponse{Success: true, Status: "deleted", Snapshot: map[string]string{"final_snapshot_id": "cloudslash-final-" + id}}, nil
		}
		if _, err := client.AddTagsToResource(ctx, &rds.AddTagsToResourceInput{
			ResourceName: out.DBInstances[0].DBInstanceArn,
			Tags:         []rdstypes.Tag{{Key: aws.String("CloudSlash:Status"), Value: aws.String("Purgatory")}},
		}); err != nil {
			return &pb.RemediateResponse{Success: false, Error: err.Error()}, nil
		}
		if _, err := client.StopDBInstance(ctx, &rds.StopDBInstanceInput{DBInstanceIdentifier: aws.String(id)}); err != nil {
			return &pb.RemediateResponse{Success: false, Error: err.Error()}, nil
		}
		return &pb.RemediateResponse{Success: true, Status: "stopped", Snapshot: map[string]string{"db_instance_id": id}}, nil
	}
}

// Validate before loading credentials or issuing any provider call. Unknown verbs
// must never fall through to a destructive default, including during preflight.
func validateRemediationAction(service, typ, action string) error {
	switch service + "/" + typ {
	case "ec2/instance":
		if action == "start" || action == "stop" || action == "terminate" || action == "delete" {
			return nil
		}
	case "ec2/volume":
		if action == "delete" {
			return nil
		}
	case "rds/db-instance":
		if action == "start" || action == "stop" || action == "delete" {
			return nil
		}
	}
	return fmt.Errorf("unsupported action %q for %s/%s", action, service, typ)
}
