package s3

import (
	"context"
	"testing"

	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	awss3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type fakeHeadObjectClient struct {
	lastInput *awss3.HeadObjectInput
	output    *awss3.HeadObjectOutput
	err       error
}

func (f *fakeHeadObjectClient) HeadObject(
	ctx context.Context,
	params *awss3.HeadObjectInput,
	optFns ...func(*awss3.Options),
) (*awss3.HeadObjectOutput, error) {
	f.lastInput = params
	return f.output, f.err
}

func TestHeadObjectReturnsMetadataWithChecksum(t *testing.T) {
	client := &fakeHeadObjectClient{
		output: &awss3.HeadObjectOutput{
			ContentLength:  int64Ptr(321),
			ChecksumSHA256: stringPtr("abc123checksumbase64=="),
		},
	}

	provider := &Provider{
		headObjectClient: client,
		bucket:           "backup-bucket",
	}

	metadata, err := provider.HeadObject(context.Background(), "path/to/object.sql", "ver-1")
	if err != nil {
		t.Fatalf("head object: %v", err)
	}

	if client.lastInput == nil {
		t.Fatalf("expected head object input to be recorded")
	}
	if value := valueString(client.lastInput.Key); value != "path/to/object.sql" {
		t.Fatalf("unexpected key: %q", value)
	}
	if value := valueString(client.lastInput.VersionId); value != "ver-1" {
		t.Fatalf("unexpected version id: %q", value)
	}
	if client.lastInput.ChecksumMode != awss3types.ChecksumModeEnabled {
		t.Fatalf("expected checksum mode enabled, got %q", client.lastInput.ChecksumMode)
	}
	if metadata.Size != 321 {
		t.Fatalf("expected size 321, got %d", metadata.Size)
	}
	if metadata.ChecksumSHA256 != "abc123checksumbase64==" {
		t.Fatalf("unexpected checksum: %q", metadata.ChecksumSHA256)
	}
}

func TestHeadObjectWithoutVersionID(t *testing.T) {
	client := &fakeHeadObjectClient{
		output: &awss3.HeadObjectOutput{
			ContentLength:  int64Ptr(64),
			ChecksumSHA256: stringPtr(""),
		},
	}

	provider := &Provider{
		headObjectClient: client,
		bucket:           "backup-bucket",
	}

	metadata, err := provider.HeadObject(context.Background(), "path/to/object.sql", "")
	if err != nil {
		t.Fatalf("head object: %v", err)
	}

	if client.lastInput == nil {
		t.Fatalf("expected head object input to be recorded")
	}
	if client.lastInput.VersionId != nil {
		t.Fatalf("expected nil version id when omitted")
	}
	if metadata.Size != 64 {
		t.Fatalf("expected size 64, got %d", metadata.Size)
	}
}

func int64Ptr(value int64) *int64 {
	return &value
}

func stringPtr(value string) *string {
	return &value
}

func valueString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
