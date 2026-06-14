package admin

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/storage/models"
)

// BucketToResponse converts a BucketResource to a BucketResponse.
func BucketToResponse(b *models.BucketResource) BucketResponse {
	return BucketResponse{
		Name:        b.Metadata.Name,
		TenantID:    b.Metadata.TenantID,
		Region:      b.Spec.Region,
		Phase:       b.Status.Phase,
		Versioning:  b.Spec.Versioning,
		ObjectCount: b.Status.ObjectCount,
		TotalSize:   b.Status.TotalSize,
		Labels:      b.Metadata.Labels,
		CreatedAt:   b.Metadata.CreatedAt,
		UpdatedAt:   b.Metadata.UpdatedAt,
	}
}

// BucketsToResponse converts a slice of BucketResource to BucketResponse slice.
func BucketsToResponse(buckets []*models.BucketResource) []BucketResponse {
	result := make([]BucketResponse, len(buckets))
	for i, b := range buckets {
		result[i] = BucketToResponse(b)
	}
	return result
}

// ObjectToResponse converts object info to an ObjectResponse.
func ObjectToResponse(obj models.ObjectInfo) ObjectResponse {
	return ObjectResponse{
		Key:          obj.Key,
		Size:         obj.Size,
		ETag:         obj.ETag,
		ContentType:  obj.ContentType,
		StorageClass: obj.StorageClass,
		LastModified: obj.LastModified,
		Metadata:     obj.UserMetadata,
	}
}

// ObjectsToResponse converts a slice of ObjectInfo to ObjectResponse slice.
func ObjectsToResponse(objects []models.ObjectInfo) []ObjectResponse {
	result := make([]ObjectResponse, len(objects))
	for i, obj := range objects {
		result[i] = ObjectToResponse(obj)
	}
	return result
}

// AccessKeyToResponse converts an AccessKey to an AccessKeyResponse.
func AccessKeyToResponse(ak *models.AccessKey) AccessKeyResponse {
	preview := ""
	if len(ak.AccessKeyID) > 8 {
		preview = ak.AccessKeyID[:4] + "..." + ak.AccessKeyID[len(ak.AccessKeyID)-4:]
	}
	return AccessKeyResponse{
		ID:          ak.AccessKeyID,
		Name:        ak.Name,
		Description: ak.Description,
		Role:        ak.Role,
		KeyPreview:  preview,
		CreatedAt:   ak.CreatedAt,
	}
}

// ShareToResponse converts a BucketShare to a BucketShareResponse.
func ShareToResponse(s *models.BucketShare) BucketShareResponse {
	resp := BucketShareResponse{
		ID:          s.ID,
		Bucket:      s.BucketName,
		GranteeType: s.GranteeType,
		GranteeID:   s.GranteeID,
		Role:        s.Role,
		CreatedAt:   s.SharedAt,
	}
	if s.ExpiresAt != nil && !s.ExpiresAt.IsZero() {
		resp.ExpiresAt = s.ExpiresAt
	}
	return resp
}

// EventToResponse converts a StorageEvent to an EventResponse.
func EventToResponse(e models.StorageEvent) EventResponse {
	return EventResponse{
		ID:        e.ID,
		Type:      e.Type,
		Bucket:    e.Bucket,
		Key:       e.Key,
		UserID:    e.UserID,
		SourceIP:  e.SourceIP,
		Timestamp: e.Timestamp,
	}
}

// EventsToResponse converts a slice of StorageEvent to EventResponse slice.
func EventsToResponse(evts []models.StorageEvent) []EventResponse {
	result := make([]EventResponse, len(evts))
	for i, e := range evts {
		result[i] = EventToResponse(e)
	}
	return result
}

// TimePtr returns a pointer to the given time (helper for optional expiry).
func TimePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
