package s3adapter

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/FrostWalk/backrest-config-backup/internal/domain/backup"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
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

	sort.Slice(objects, func(i, j int) bool {
		if objects[i].LastModified == nil {
			return false
		}
		if objects[j].LastModified == nil {
			return true
		}
		return objects[i].LastModified.After(*objects[j].LastModified)
	})

	key := aws.ToString(objects[0].Key)
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

func (s *Storage) CleanupBackups(ctx context.Context, keepObjectKey string) (int, error) {
	deletedCount, err := s.cleanupUsingObjectVersions(ctx, keepObjectKey)
	if err == nil {
		return deletedCount, nil
	}
	// Fallback for providers without ListObjectVersions support.
	return s.cleanupUsingObjectList(ctx, keepObjectKey)
}

func (s *Storage) listAllObjects(ctx context.Context) ([]s3typesObject, error) {
	var (
		token   *string
		objects []s3typesObject
	)

	for {
		output, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(s.bucket),
			Prefix:            aws.String(s.fullPrefix()),
			ContinuationToken: token,
		})
		if err != nil {
			return nil, fmt.Errorf("list objects under prefix %q: %w", s.fullPrefix(), err)
		}

		for _, item := range output.Contents {
			key := aws.ToString(item.Key)
			if strings.HasSuffix(key, ".json.age") {
				objects = append(objects, s3typesObject{
					Key:          item.Key,
					LastModified: item.LastModified,
				})
			}
		}

		if !aws.ToBool(output.IsTruncated) {
			break
		}
		token = output.NextContinuationToken
	}

	return objects, nil
}

func (s *Storage) fullPrefix() string {
	if s.prefix == "" {
		return ""
	}
	return s.prefix + "/"
}

func (s *Storage) cleanupUsingObjectVersions(ctx context.Context, keepObjectKey string) (int, error) {
	var (
		keyMarker       *string
		versionIDMarker *string
		objectsToDelete []s3types.ObjectIdentifier
	)

	for {
		output, err := s.client.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{
			Bucket:          aws.String(s.bucket),
			Prefix:          aws.String(s.fullPrefix()),
			KeyMarker:       keyMarker,
			VersionIdMarker: versionIDMarker,
		})
		if err != nil {
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

		if !aws.ToBool(output.IsTruncated) {
			break
		}
		keyMarker = output.NextKeyMarker
		versionIDMarker = output.NextVersionIdMarker
	}

	if len(objectsToDelete) == 0 {
		return 0, nil
	}

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

func (s *Storage) cleanupUsingObjectList(ctx context.Context, keepObjectKey string) (int, error) {
	objects, err := s.listAllObjects(ctx)
	if err != nil {
		return 0, err
	}

	deletedCount := 0
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

		waiter := s3.NewObjectNotExistsWaiter(s.client)
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

type s3typesObject struct {
	Key          *string
	LastModified *time.Time
}
