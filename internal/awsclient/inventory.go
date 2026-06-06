package awsclient

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"

	"sesame/internal/domain"
)

type InventoryProvider struct {
	Region string
	EC2    EC2API
	SSM    SSMAPI
}

type EC2API interface {
	DescribeInstances(context.Context, *ec2.DescribeInstancesInput, ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
}

type SSMAPI interface {
	DescribeInstanceInformation(context.Context, *ssm.DescribeInstanceInformationInput, ...func(*ssm.Options)) (*ssm.DescribeInstanceInformationOutput, error)
}

func (p InventoryProvider) ListInstances(ctx context.Context) ([]domain.Instance, []domain.Warning, error) {
	instances, err := p.listEC2(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("describe EC2 instances: %w", err)
	}

	ssmInfo, err := p.describeSSM(ctx)
	if err != nil {
		for i := range instances {
			instances[i].SSMStatus = domain.SSMStatusUnknown
		}
		return instances, []domain.Warning{{
			Code:    "ssm-describe-instance-information-failed",
			Message: err.Error(),
		}}, nil
	}

	mergeSSM(instances, ssmInfo)
	return instances, nil, nil
}

func (p InventoryProvider) GetInstance(ctx context.Context, instanceID string) (domain.Instance, error) {
	instances, err := p.listEC2(ctx, []ec2types.Filter{{
		Name:   aws.String("instance-id"),
		Values: []string{instanceID},
	}})
	if err != nil {
		return domain.Instance{}, fmt.Errorf("describe EC2 instance: %w", err)
	}
	if len(instances) == 0 {
		return domain.Instance{}, fmt.Errorf("%w: %s", domain.ErrInstanceNotFound, instanceID)
	}

	ssmInfo, err := p.describeSSM(ctx)
	if err != nil {
		instances[0].SSMStatus = domain.SSMStatusError
		return instances[0], nil
	}
	mergeSSM(instances, ssmInfo)
	return instances[0], nil
}

func (p InventoryProvider) listEC2(ctx context.Context, filters []ec2types.Filter) ([]domain.Instance, error) {
	paginator := ec2.NewDescribeInstancesPaginator(p.EC2, &ec2.DescribeInstancesInput{
		Filters: filters,
	})

	var out []domain.Instance
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, reservation := range page.Reservations {
			for _, inst := range reservation.Instances {
				out = append(out, p.mapEC2Instance(inst))
			}
		}
	}
	return out, nil
}

func (p InventoryProvider) describeSSM(ctx context.Context) (map[string]ssmtypes.InstanceInformation, error) {
	paginator := ssm.NewDescribeInstanceInformationPaginator(p.SSM, &ssm.DescribeInstanceInformationInput{})
	out := map[string]ssmtypes.InstanceInformation{}
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, info := range page.InstanceInformationList {
			out[aws.ToString(info.InstanceId)] = info
		}
	}
	return out, nil
}

func (p InventoryProvider) mapEC2Instance(inst ec2types.Instance) domain.Instance {
	tags := map[string]string{}
	for _, tag := range inst.Tags {
		tags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
	}
	var createdAt int64
	if inst.LaunchTime != nil {
		createdAt = inst.LaunchTime.Unix()
	}
	var az string
	if inst.Placement != nil {
		az = aws.ToString(inst.Placement.AvailabilityZone)
	}
	securityGroups := make([]domain.SecurityGroup, 0, len(inst.SecurityGroups))
	for _, sg := range inst.SecurityGroups {
		securityGroups = append(securityGroups, domain.SecurityGroup{
			ID:   aws.ToString(sg.GroupId),
			Name: aws.ToString(sg.GroupName),
		})
	}
	return domain.Instance{
		ID:             aws.ToString(inst.InstanceId),
		Name:           tags["Name"],
		State:          string(inst.State.Name),
		Type:           string(inst.InstanceType),
		AMIID:          aws.ToString(inst.ImageId),
		PrivateIP:      aws.ToString(inst.PrivateIpAddress),
		PublicIP:       aws.ToString(inst.PublicIpAddress),
		Region:         p.Region,
		AZ:             az,
		SSMStatus:      domain.SSMStatusNotManaged,
		CreatedAt:      createdAt,
		SecurityGroups: securityGroups,
		Tags:           tags,
	}
}

func mergeSSM(instances []domain.Instance, ssmInfo map[string]ssmtypes.InstanceInformation) {
	for i := range instances {
		info, ok := ssmInfo[instances[i].ID]
		if !ok {
			instances[i].SSMStatus = domain.SSMStatusNotManaged
			continue
		}
		instances[i].SSMStatus = mapPingStatus(info.PingStatus)
		instances[i].Agent = domain.AgentInfo{
			Version:      aws.ToString(info.AgentVersion),
			PlatformType: string(info.PlatformType),
		}
		if info.LastPingDateTime != nil {
			instances[i].Agent.LastPingUnixTime = info.LastPingDateTime.Unix()
		}
	}
}

func mapPingStatus(status ssmtypes.PingStatus) domain.SSMStatus {
	switch strings.ToLower(string(status)) {
	case "online":
		return domain.SSMStatusOnline
	case "connectionlost":
		return domain.SSMStatusConnectionLost
	default:
		return domain.SSMStatusUnknown
	}
}
