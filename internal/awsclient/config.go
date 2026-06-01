package awsclient

import (
	"context"
	"errors"
	"os"
	"strings"

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

type EnvConfig struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Region          string
	DefaultRegion   string
	Profile         string
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
	auth := ResolveAuthContext(input, EnvConfig{
		AccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
		SessionToken:    os.Getenv("AWS_SESSION_TOKEN"),
		Region:          os.Getenv("AWS_REGION"),
		DefaultRegion:   os.Getenv("AWS_DEFAULT_REGION"),
		Profile:         os.Getenv("AWS_PROFILE"),
	})

	opts := make([]func(*config.LoadOptions) error, 0, 2)
	if auth.Region != "" {
		opts = append(opts, config.WithRegion(auth.Region))
	}

	if auth.Mode == domain.AuthModeProfileActive {
		opts = append(opts, config.WithSharedConfigProfile(auth.Profile))
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

func ResolveAuthContext(input ConfigInput, env EnvConfig) domain.AuthContext {
	region := firstNonEmpty(input.Region, env.Region, env.DefaultRegion)
	if strings.TrimSpace(env.AccessKeyID) != "" && strings.TrimSpace(env.SecretAccessKey) != "" {
		return domain.AuthContext{
			Mode:   domain.AuthModeEnvActive,
			Region: region,
		}
	}

	return domain.AuthContext{
		Mode:    domain.AuthModeProfileActive,
		Profile: firstNonEmpty(input.Profile, env.Profile, "default"),
		Region:  region,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
