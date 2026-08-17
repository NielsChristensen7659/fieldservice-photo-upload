package fieldupload

import (
	"context"
	"testing"
	"time"
)

type recordingSigner struct {
	input PresignPutInput
	key   string
}

func (s *recordingSigner) PresignPut(_ context.Context, _, key string, in PresignPutInput) (PresignResult, error) {
	s.key, s.input = key, in
	return PresignResult{URL: "https://uploads.example/signed"}, nil
}

func TestRequestPhotoUploadDispatchDecision(t *testing.T) {
	tests := []struct {
		name       string
		status     string
		wantFollow bool
	}{
		{name: "technician still travelling", status: "en_route", wantFollow: true},
		{name: "technician at job", status: "on_site", wantFollow: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signer := &recordingSigner{}
			service := UploadService{Signer: signer, Bucket: "field-assets", Now: func() time.Time {
				return time.Date(2026, 8, 17, 9, 30, 0, 0, time.UTC)
			}}
			got, err := service.RequestPhotoUpload(context.Background(), UploadRequest{
				WorkOrderID: "WO-1042", TechnicianID: "tech-7", DispatchStatus: tt.status,
				Filename: "panel.jpg", ContentType: "image/jpeg", SizeBytes: 2048,
			})
			if err != nil {
				t.Fatal(err)
			}
			if got.FollowUpRequired != tt.wantFollow {
				t.Fatalf("follow_up_required = %v, want %v", got.FollowUpRequired, tt.wantFollow)
			}
			wantKey := "work-orders/WO-1042/photos/2026/08/17/tech-7-"
			if len(signer.key) <= len(wantKey) || signer.key[:len(wantKey)] != wantKey {
				t.Fatalf("object key %q does not start with %q", signer.key, wantKey)
			}
			if signer.input.MaxBytes != 2048 || signer.input.ExpiresSeconds != 600 || signer.input.IdempotencyKey == "" {
				t.Fatalf("unexpected presign boundary: %+v", signer.input)
			}
		})
	}
}
