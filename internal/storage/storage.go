package storage

import (
	"context"
	"time"
)

type ObjectInfo struct {
	Key          string
	Size         int64
	LastModified time.Time
	VersionID    string
}

type ObjectMetadata struct {
	Size           int64
	ChecksumSHA256 string
}

type Provider interface {
	Put(ctx context.Context, key string, localPath string) error
	Get(ctx context.Context, key string, versionID string, localPath string) error
	ListObjects(ctx context.Context, prefix string) ([]ObjectInfo, error)
	ListObjectVersions(ctx context.Context, prefix string) ([]ObjectInfo, error)
	HeadObject(ctx context.Context, key string, versionID string) (ObjectMetadata, error)
}
