package fieldupload

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const MaxPhotoBytes int64 = 12 << 20

var safeID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type UploadRequest struct {
	WorkOrderID    string `json:"work_order_id"`
	TechnicianID   string `json:"technician_id"`
	DispatchStatus string `json:"dispatch_status"`
	Filename       string `json:"filename"`
	ContentType    string `json:"content_type"`
	SizeBytes      int64  `json:"size_bytes"`
}

type UploadDecision struct {
	UploadURL        string `json:"upload_url"`
	Method           string `json:"method"`
	ObjectKey        string `json:"object_key"`
	DispatchStatus   string `json:"dispatch_status"`
	FollowUpRequired bool   `json:"follow_up_required"`
}

type Signer interface {
	PresignPut(context.Context, string, string, PresignPutInput) (PresignResult, error)
}

type UploadService struct {
	Signer Signer
	Bucket string
	Now    func() time.Time
}

func (s UploadService) RequestPhotoUpload(ctx context.Context, in UploadRequest) (UploadDecision, error) {
	if !safeID.MatchString(in.WorkOrderID) || !safeID.MatchString(in.TechnicianID) {
		return UploadDecision{}, errors.New("work_order_id and technician_id must use letters, digits, underscore, or hyphen")
	}
	if in.DispatchStatus != "en_route" && in.DispatchStatus != "on_site" {
		return UploadDecision{}, errors.New("dispatch_status must be en_route or on_site")
	}
	if in.ContentType != "image/jpeg" && in.ContentType != "image/png" {
		return UploadDecision{}, errors.New("content_type must be image/jpeg or image/png")
	}
	if in.SizeBytes <= 0 || in.SizeBytes > MaxPhotoBytes {
		return UploadDecision{}, fmt.Errorf("size_bytes must be between 1 and %d", MaxPhotoBytes)
	}
	ext := strings.ToLower(filepath.Ext(in.Filename))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
		return UploadDecision{}, errors.New("filename must end in .jpg, .jpeg, or .png")
	}
	now := s.Now().UTC()
	digest := sha256.Sum256([]byte(in.WorkOrderID + "\x00" + in.TechnicianID + "\x00" + in.Filename + "\x00" + fmt.Sprint(in.SizeBytes)))
	key := fmt.Sprintf("work-orders/%s/photos/%s/%s-%s%s", in.WorkOrderID, now.Format("2006/01/02"), in.TechnicianID, hex.EncodeToString(digest[:8]), ext)
	idempotencyKey := "photo-upload-" + hex.EncodeToString(digest[:])

	signed, err := s.Signer.PresignPut(ctx, s.Bucket, key, PresignPutInput{
		ContentType: in.ContentType, MaxBytes: in.SizeBytes, ExpiresSeconds: 600, IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		return UploadDecision{}, err
	}
	return UploadDecision{
		UploadURL: signed.URL, Method: "PUT", ObjectKey: key, DispatchStatus: in.DispatchStatus,
		FollowUpRequired: in.DispatchStatus == "en_route",
	}, nil
}
