package awsclient

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type fakeRegionEC2Client struct {
	input *ec2.DescribeRegionsInput
	out   *ec2.DescribeRegionsOutput
	err   error
}

func (f *fakeRegionEC2Client) DescribeRegions(_ context.Context, input *ec2.DescribeRegionsInput, _ ...func(*ec2.Options)) (*ec2.DescribeRegionsOutput, error) {
	f.input = input
	return f.out, f.err
}

func TestRegionProviderListsSortedEnabledRegions(t *testing.T) {
	client := &fakeRegionEC2Client{out: &ec2.DescribeRegionsOutput{
		Regions: []ec2types.Region{
			{RegionName: aws.String("eu-west-1")},
			{RegionName: aws.String("eu-central-1")},
			{RegionName: aws.String("")},
			{RegionName: aws.String("us-east-1")},
		},
	}}
	provider := RegionProvider{EC2: client}

	got, err := provider.ListRegions(context.Background())
	if err != nil {
		t.Fatalf("ListRegions returned error: %v", err)
	}
	want := []string{"eu-central-1", "eu-west-1", "us-east-1"}
	if len(got) != len(want) {
		t.Fatalf("expected %d regions, got %#v", len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("region %d = %q, want %q; all=%#v", i, got[i], want[i], got)
		}
	}
	if client.input == nil {
		t.Fatal("expected DescribeRegions input")
	}
	if client.input.AllRegions != nil {
		t.Fatalf("expected AllRegions to be unset, got %v", *client.input.AllRegions)
	}
}

func TestRegionProviderWrapsDescribeRegionsError(t *testing.T) {
	provider := RegionProvider{EC2: &fakeRegionEC2Client{err: errors.New("denied")}}

	_, err := provider.ListRegions(context.Background())
	if err == nil || err.Error() != "describe regions: denied" {
		t.Fatalf("expected wrapped error, got %v", err)
	}
}
