package s3adapter

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/FrostWalk/backrest-config-backup/internal/domain/backup"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

type Storage struct {
	client *s3.Client
	bucket string
	prefix string
}

func NewStorage(client *s3.Client, bucket, prefix string) *Storage {
	return &Storage{
		client: client,
		bucket: strings.TrimSpace(bucket),
		prefix: strings.Trim(strings.TrimSpace(prefix), "/"),
	}
}

func (s *Storage) GetLatestBackup(ctx context.Context) (*backup.StoredBackup, error) {
	objects, err := s.listAllObjects(ctx)
	if err != nil {
		return nil, err
	}
	if len(objects) == 0 {
		return nil, nil
	}

	latest := objects[0]
	for _, object := range objects[1:] {
		if object.LastModified != nil && (latest.LastModified == nil || object.LastModified.After(*latest.LastModified)) {
			latest = object
		}
	}

	key := aws.ToString(latest.Key)
	head, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("head object %q: %w", key, err)
	}

	return &backup.StoredBackup{
		ObjectKey: key,
		Hash:      head.Metadata[backup.HashMetadataKey],
	}, nil
}

func (s *Storage) UploadBackup(ctx context.Context, objectKey string, encrypted []byte, configHash string) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(objectKey),
		Body:        bytes.NewReader(encrypted),
		ContentType: aws.String("application/octet-stream"),
		Metadata: map[string]string{
			backup.HashMetadataKey: configHash,
		},
	})
	if err != nil {
		return fmt.Errorf("put object %q: %w", objectKey, err)
	}
	return nil
}

func (s *Storage) listAllObjects(ctx context.Context) ([]s3types.Object, error) {
	pages := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(s.fullPrefix()),
	})
	var objects []s3types.Object
	for pages.HasMorePages() {
		output, err := pages.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list objects under prefix %q: %w", s.fullPrefix(), err)
		}

		for _, item := range output.Contents {
			key := aws.ToString(item.Key)
			if strings.HasSuffix(key, ".json.age") {
				objects = append(objects, item)
			}
		}
	}

	return objects, nil
}

func (s *Storage) fullPrefix() string {
	if s.prefix == "" {
		return ""
	}
	return s.prefix + "/"
}

func (s *Storage) CleanupBackups(ctx context.Context, keepObjectKey string) (int, error) {
	pages := s3.NewListObjectVersionsPaginator(s.client, &s3.ListObjectVersionsInput{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(s.fullPrefix()),
	})
	var objectsToDelete []s3types.ObjectIdentifier
	for pages.HasMorePages() {
		output, err := pages.NextPage(ctx)
		if err != nil {
			// Only unsupported version listing permits falling back to unversioned cleanup.
			if versionListingUnsupported(err) {
				return s.cleanupUsingObjectList(ctx, keepObjectKey)
			}
			return 0, fmt.Errorf("list object versions under prefix %q: %w", s.fullPrefix(), err)
		}

		for _, version := range output.Versions {
			key := aws.ToString(version.Key)
			if key == keepObjectKey || !strings.HasSuffix(key, ".json.age") {
				continue
			}
			objectsToDelete = append(objectsToDelete, s3types.ObjectIdentifier{
				Key:       version.Key,
				VersionId: version.VersionId,
			})
		}

		for _, marker := range output.DeleteMarkers {
			key := aws.ToString(marker.Key)
			if key == keepObjectKey || !strings.HasSuffix(key, ".json.age") {
				continue
			}
			objectsToDelete = append(objectsToDelete, s3types.ObjectIdentifier{
				Key:       marker.Key,
				VersionId: marker.VersionId,
			})
		}
	}

	// Finish listing before deleting so mutations cannot invalidate pagination markers.
	deletedCount := 0
	for _, object := range objectsToDelete {
		_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket:    aws.String(s.bucket),
			Key:       object.Key,
			VersionId: object.VersionId,
		})
		if err != nil {
			return deletedCount, fmt.Errorf("delete object %q version %q: %w", aws.ToString(object.Key), aws.ToString(object.VersionId), err)
		}
		deletedCount++
	}
	return deletedCount, nil
}

func versionListingUnsupported(err error) bool {
	if apiErr, ok := errors.AsType[smithy.APIError](err); ok {
		switch apiErr.ErrorCode() {
		case "NotImplemented", "MethodNotAllowed":
			return true
		}
	}
	if responseErr, ok := errors.AsType[*smithyhttp.ResponseError](err); ok {
		return responseErr.HTTPStatusCode() == http.StatusNotImplemented || responseErr.HTTPStatusCode() == http.StatusMethodNotAllowed
	}
	return false
}

func (s *Storage) cleanupUsingObjectList(ctx context.Context, keepObjectKey string) (int, error) {
	objects, err := s.listAllObjects(ctx)
	if err != nil {
		return 0, err
	}

	deletedCount := 0
	waiter := s3.NewObjectNotExistsWaiter(s.client)
	for _, object := range objects {
		key := aws.ToString(object.Key)
		if key == keepObjectKey {
			continue
		}

		if _, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(s.bucket),
			Key:    aws.String(key),
		}); err != nil {
			return deletedCount, fmt.Errorf("delete object %q: %w", key, err)
		}

		if err := waiter.Wait(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(s.bucket),
			Key:    aws.String(key),
		}, 15*time.Second); err != nil {
			return deletedCount, fmt.Errorf("waiting object deletion %q: %w", key, err)
		}
		deletedCount++
	}

	return deletedCount, nil
}
