package s3

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"

	"sloth/internal/config"
	"sloth/internal/storage"
)

type Provider struct {
	client *awss3.Client
	bucket string
}

func NewProvider(cfg config.StorageConfig) (*Provider, error) {
	region := cfg.Region
	if region == "" || region == "auto" {
		region = "us-east-1"
	}

	awsCfg := aws.Config{
		Region:      region,
		Credentials: credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		EndpointResolverWithOptions: aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{URL: cfg.Endpoint}, nil
		}),
	}

	client := awss3.NewFromConfig(awsCfg, func(options *awss3.Options) {
		options.UsePathStyle = true
	})

	return &Provider{client: client, bucket: cfg.Bucket}, nil
}

func (p *Provider) Put(ctx context.Context, key string, localPath string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open backup artifact: %w", err)
	}
	defer file.Close()

	_, err = p.client.PutObject(ctx, &awss3.PutObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    aws.String(key),
		Body:   file,
	})
	if err != nil {
		return fmt.Errorf("put object %s: %w", key, err)
	}

	return nil
}

func (p *Provider) Get(ctx context.Context, key string, versionID string, localPath string) error {
	input := &awss3.GetObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    aws.String(key),
	}
	if versionID != "" {
		input.VersionId = aws.String(versionID)
	}

	output, err := p.client.GetObject(ctx, input)
	if err != nil {
		return fmt.Errorf("get object %s: %w", key, err)
	}
	defer output.Body.Close()

	file, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("create local restore file: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, output.Body); err != nil {
		return fmt.Errorf("write restore file %s: %w", localPath, err)
	}

	return nil
}

func (p *Provider) ListObjects(ctx context.Context, prefix string) ([]storage.ObjectInfo, error) {
	paginator := awss3.NewListObjectsV2Paginator(p.client, &awss3.ListObjectsV2Input{
		Bucket: aws.String(p.bucket),
		Prefix: aws.String(prefix),
	})

	objects := []storage.ObjectInfo{}
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list objects: %w", err)
		}
		for _, obj := range page.Contents {
			info := storage.ObjectInfo{Key: aws.ToString(obj.Key), Size: aws.ToInt64(obj.Size)}
			if obj.LastModified != nil {
				info.LastModified = *obj.LastModified
			}
			objects = append(objects, info)
		}
	}

	return objects, nil
}

func (p *Provider) ListObjectVersions(ctx context.Context, prefix string) ([]storage.ObjectInfo, error) {
	paginator := awss3.NewListObjectVersionsPaginator(p.client, &awss3.ListObjectVersionsInput{
		Bucket: aws.String(p.bucket),
		Prefix: aws.String(prefix),
	})

	versions := []storage.ObjectInfo{}
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list object versions: %w", err)
		}
		for _, version := range page.Versions {
			info := storage.ObjectInfo{
				Key:       aws.ToString(version.Key),
				Size:      aws.ToInt64(version.Size),
				VersionID: aws.ToString(version.VersionId),
			}
			if version.LastModified != nil {
				info.LastModified = *version.LastModified
			}
			versions = append(versions, info)
		}
	}

	return versions, nil
}
