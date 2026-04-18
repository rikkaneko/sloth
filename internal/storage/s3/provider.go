package s3

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	awss3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	"sloth/internal/config"
	"sloth/internal/storage"
	"sloth/internal/ui"
)

type Provider struct {
	client           *awss3.Client
	headObjectClient headObjectClient
	bucket           string
}

type headObjectClient interface {
	HeadObject(ctx context.Context, params *awss3.HeadObjectInput, optFns ...func(*awss3.Options)) (*awss3.HeadObjectOutput, error)
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

	return &Provider{
		client:           client,
		headObjectClient: client,
		bucket:           cfg.Bucket,
	}, nil
}

func (p *Provider) Put(ctx context.Context, key string, localPath string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open backup artifact: %w", err)
	}
	defer file.Close()

	ui.Debugf("s3::PutObject {bucket:%q, key:%q, file:%q}", p.bucket, key, localPath)
	output, err := p.client.PutObject(ctx, &awss3.PutObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    aws.String(key),
		Body:   file,
	})
	if err != nil {
		return fmt.Errorf("put object %s: %w", key, err)
	}
	ui.Debugf(
		"s3::PutObject response {etag:%q, version_id:%q}",
		aws.ToString(output.ETag),
		aws.ToString(output.VersionId),
	)

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

	ui.Debugf("s3::GetObject {bucket:%q, key:%q, version_id:%q, dest:%q}", p.bucket, key, versionID, localPath)
	output, err := p.client.GetObject(ctx, input)
	if err != nil {
		return fmt.Errorf("get object %s: %w", key, err)
	}
	defer output.Body.Close()
	ui.Debugf("s3::GetObject response {content_length:%d}", output.ContentLength)

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
	ui.Debugf("s3::ListObjectsV2 {bucket:%q, prefix:%q}", p.bucket, prefix)
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
		ui.Debugf("s3::ListObjectsV2 page response {key_count:%d, truncated:%t}", aws.ToInt32(page.KeyCount), page.IsTruncated)
		for _, obj := range page.Contents {
			info := storage.ObjectInfo{Key: aws.ToString(obj.Key), Size: aws.ToInt64(obj.Size)}
			if obj.LastModified != nil {
				info.LastModified = *obj.LastModified
			}
			objects = append(objects, info)
		}
	}
	ui.Debugf("s3::ListObjectsV2 result {objects:%d}", len(objects))

	return objects, nil
}

func (p *Provider) ListObjectVersions(ctx context.Context, prefix string) ([]storage.ObjectInfo, error) {
	ui.Debugf("s3::ListObjectVersions {bucket:%q, prefix:%q}", p.bucket, prefix)
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
		ui.Debugf("s3::ListObjectVersions page response {versions:%d, truncated:%t}", len(page.Versions), page.IsTruncated)
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
	ui.Debugf("s3::ListObjectVersions result {versions:%d}", len(versions))

	return versions, nil
}

func (p *Provider) HeadObject(ctx context.Context, key string, versionID string) (storage.ObjectMetadata, error) {
	input := &awss3.HeadObjectInput{
		Bucket:       aws.String(p.bucket),
		Key:          aws.String(key),
		ChecksumMode: awss3types.ChecksumModeEnabled,
	}
	if versionID != "" {
		input.VersionId = aws.String(versionID)
	}

	ui.Debugf("s3::HeadObject {bucket:%q, key:%q, version_id:%q, checksum_mode:%q}", p.bucket, key, versionID, awss3types.ChecksumModeEnabled)
	output, err := p.headObjectClient.HeadObject(ctx, input)
	if err != nil {
		return storage.ObjectMetadata{}, fmt.Errorf("head object %s: %w", key, err)
	}

	metadata := storage.ObjectMetadata{
		Size:           aws.ToInt64(output.ContentLength),
		ChecksumSHA256: aws.ToString(output.ChecksumSHA256),
	}
	ui.Debugf("s3::HeadObject response {content_length:%d, checksum_sha256:%q}", metadata.Size, metadata.ChecksumSHA256)
	return metadata, nil
}
