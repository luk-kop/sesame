package awsclient

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"sesame/internal/domain"
)

type IdentityProvider struct {
	Client *sts.Client
}

func (p IdentityProvider) GetCallerIdentity(ctx context.Context) (domain.Identity, error) {
	out, err := p.Client.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return domain.Identity{}, err
	}
	return domain.Identity{
		Account: aws.ToString(out.Account),
		ARN:     aws.ToString(out.Arn),
		UserID:  aws.ToString(out.UserId),
	}, nil
}
