package awsclient

import (
	"context"
	"errors"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"sesame/internal/domain"
)

type ConfigInput struct {
	Profile string
	Region  string
}

type Clients struct {
	Auth domain.AuthContext
	EC2  *ec2.Client
	SSM  *ssm.Client
	STS  *sts.Client
}

func NewClients(ctx context.Context, input ConfigInput) (*Clients, error) {
	auth, cfg, err := LoadConfig(ctx, input)
	if err != nil {
		return nil, err
	}
	return &Clients{
		Auth: auth,
		EC2:  ec2.NewFromConfig(cfg),
		SSM:  ssm.NewFromConfig(cfg),
		STS:  sts.NewFromConfig(cfg),
	}, nil
}

func LoadConfig(ctx context.Context, input ConfigInput) (domain.AuthContext, aws.Config, error) {
	envActive := os.Getenv("AWS_ACCESS_KEY_ID") != "" && os.Getenv("AWS_SECRET_ACCESS_KEY") != ""
	region := firstNonEmpty(input.Region, os.Getenv("AWS_REGION"), os.Getenv("AWS_DEFAULT_REGION"))

	opts := make([]func(*config.LoadOptions) error, 0, 2)
	if region != "" {
		opts = append(opts, config.WithRegion(region))
	}

	auth := domain.AuthContext{Region: region}
	if envActive {
		auth.Mode = domain.AuthModeEnvActive
	} else {
		profile := firstNonEmpty(input.Profile, os.Getenv("AWS_PROFILE"), "default")
		auth.Mode = domain.AuthModeProfileActive
		auth.Profile = profile
		opts = append(opts, config.WithSharedConfigProfile(profile))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return auth, aws.Config{}, err
	}
	if cfg.Region == "" {
		return auth, aws.Config{}, errors.New("missing AWS region")
	}
	auth.Region = cfg.Region

	return auth, cfg, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
