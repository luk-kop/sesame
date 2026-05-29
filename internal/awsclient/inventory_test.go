package awsclient

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"

	"sesame/internal/domain"
)

type fakeEC2Client struct {
	out *ec2.DescribeInstancesOutput
	err error
}

func (f fakeEC2Client) DescribeInstances(context.Context, *ec2.DescribeInstancesInput, ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	return f.out, f.err
}

type fakeSSMClient struct {
	out *ssm.DescribeInstanceInformationOutput
	err error
}

func (f fakeSSMClient) DescribeInstanceInformation(context.Context, *ssm.DescribeInstanceInformationInput, ...func(*ssm.Options)) (*ssm.DescribeInstanceInformationOutput, error) {
	return f.out, f.err
}

func TestMapEC2Instance(t *testing.T) {
	provider := InventoryProvider{Region: "eu-central-1"}
	got := provider.mapEC2Instance(ec2types.Instance{
		InstanceId:       aws.String("i-123"),
		InstanceType:     ec2types.InstanceTypeT3Micro,
		PrivateIpAddress: aws.String("10.0.0.10"),
		PublicIpAddress:  aws.String("18.1.2.3"),
		State:            &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning},
		Tags: []ec2types.Tag{
			{Key: aws.String("Name"), Value: aws.String("api")},
			{Key: aws.String("Environment"), Value: aws.String("prod")},
		},
	})

	if got.ID != "i-123" || got.Name != "api" || got.State != "running" || got.Type != "t3.micro" {
		t.Fatalf("unexpected mapped instance: %#v", got)
	}
	if got.Region != "eu-central-1" || got.SSMStatus != domain.SSMStatusNotManaged {
		t.Fatalf("expected region and default not-managed SSM status, got %#v", got)
	}
	if got.Tags["Environment"] != "prod" {
		t.Fatalf("expected tags to be preserved, got %#v", got.Tags)
	}
}

func TestListInstancesMergesEC2AndSSM(t *testing.T) {
	provider := InventoryProvider{
		Region: "eu-central-1",
		EC2: fakeEC2Client{out: &ec2.DescribeInstancesOutput{
			Reservations: []ec2types.Reservation{{
				Instances: []ec2types.Instance{{
					InstanceId: aws.String("i-1"),
					State:      &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning},
					Tags:       []ec2types.Tag{{Key: aws.String("Name"), Value: aws.String("api")}},
				}},
			}},
		}},
		SSM: fakeSSMClient{out: &ssm.DescribeInstanceInformationOutput{
			InstanceInformationList: []ssmtypes.InstanceInformation{{
				InstanceId: aws.String("i-1"),
				PingStatus: ssmtypes.PingStatusOnline,
			}},
		}},
	}

	instances, warnings, err := provider.ListInstances(context.Background())
	if err != nil {
		t.Fatalf("ListInstances returned error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %#v", warnings)
	}
	if len(instances) != 1 || instances[0].ID != "i-1" || instances[0].SSMStatus != domain.SSMStatusOnline {
		t.Fatalf("expected merged online instance, got %#v", instances)
	}
}

func TestListInstancesSSMErrorIsPartialSuccess(t *testing.T) {
	provider := InventoryProvider{
		Region: "eu-central-1",
		EC2: fakeEC2Client{out: &ec2.DescribeInstancesOutput{
			Reservations: []ec2types.Reservation{{
				Instances: []ec2types.Instance{{
					InstanceId: aws.String("i-1"),
					State:      &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning},
				}},
			}},
		}},
		SSM: fakeSSMClient{err: errors.New("access denied")},
	}

	instances, warnings, err := provider.ListInstances(context.Background())
	if err != nil {
		t.Fatalf("expected partial success, got error: %v", err)
	}
	if len(instances) != 1 || instances[0].SSMStatus != domain.SSMStatusUnknown {
		t.Fatalf("expected instance with unknown SSM status, got %#v", instances)
	}
	if len(warnings) != 1 || warnings[0].Code != "ssm-describe-instance-information-failed" {
		t.Fatalf("expected SSM warning, got %#v", warnings)
	}
}

func TestListInstancesEC2ErrorIsFatal(t *testing.T) {
	provider := InventoryProvider{
		EC2: fakeEC2Client{err: errors.New("ec2 denied")},
		SSM: fakeSSMClient{},
	}

	_, _, err := provider.ListInstances(context.Background())
	if err == nil {
		t.Fatal("expected EC2 error")
	}
}

func TestGetInstanceSSMErrorReturnsInstanceWithErrorStatus(t *testing.T) {
	provider := InventoryProvider{
		Region: "eu-central-1",
		EC2: fakeEC2Client{out: &ec2.DescribeInstancesOutput{
			Reservations: []ec2types.Reservation{{
				Instances: []ec2types.Instance{{
					InstanceId: aws.String("i-1"),
					State:      &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning},
				}},
			}},
		}},
		SSM: fakeSSMClient{err: errors.New("ssm denied")},
	}

	got, err := provider.GetInstance(context.Background(), "i-1")
	if err != nil {
		t.Fatalf("expected SSM error to be represented in status, got error: %v", err)
	}
	if got.ID != "i-1" || got.SSMStatus != domain.SSMStatusError {
		t.Fatalf("expected instance with SSM error status, got %#v", got)
	}
}

func TestGetInstanceNotFoundUsesSentinel(t *testing.T) {
	provider := InventoryProvider{
		EC2: fakeEC2Client{out: &ec2.DescribeInstancesOutput{}},
		SSM: fakeSSMClient{},
	}

	_, err := provider.GetInstance(context.Background(), "i-missing")
	if !errors.Is(err, domain.ErrInstanceNotFound) {
		t.Fatalf("expected ErrInstanceNotFound, got %v", err)
	}
}

func TestMergeSSMMarksOnlineAndAgentInfo(t *testing.T) {
	ping := time.Unix(1700000000, 0)
	instances := []domain.Instance{
		{ID: "i-1", SSMStatus: domain.SSMStatusNotManaged},
		{ID: "i-2", SSMStatus: domain.SSMStatusNotManaged},
	}

	mergeSSM(instances, map[string]ssmtypes.InstanceInformation{
		"i-1": {
			InstanceId:       aws.String("i-1"),
			PingStatus:       ssmtypes.PingStatusOnline,
			AgentVersion:     aws.String("3.2.1"),
			LastPingDateTime: &ping,
			PlatformType:     ssmtypes.PlatformTypeLinux,
		},
	})

	if instances[0].SSMStatus != domain.SSMStatusOnline {
		t.Fatalf("expected i-1 online, got %s", instances[0].SSMStatus)
	}
	if instances[0].Agent.Version != "3.2.1" || instances[0].Agent.LastPingUnixTime != 1700000000 || instances[0].Agent.PlatformType != "Linux" {
		t.Fatalf("expected agent info, got %#v", instances[0].Agent)
	}
	if instances[1].SSMStatus != domain.SSMStatusNotManaged {
		t.Fatalf("expected missing SSM info to remain not-managed, got %s", instances[1].SSMStatus)
	}
}

func TestMapPingStatus(t *testing.T) {
	tests := []struct {
		status ssmtypes.PingStatus
		want   domain.SSMStatus
	}{
		{status: ssmtypes.PingStatusOnline, want: domain.SSMStatusOnline},
		{status: ssmtypes.PingStatusConnectionLost, want: domain.SSMStatusConnectionLost},
		{status: ssmtypes.PingStatusInactive, want: domain.SSMStatusUnknown},
	}

	for _, tt := range tests {
		if got := mapPingStatus(tt.status); got != tt.want {
			t.Fatalf("mapPingStatus(%q) = %q, want %q", tt.status, got, tt.want)
		}
	}
}
