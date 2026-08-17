package fieldupload

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPresignPutDecodesEnvelopeBeforeHTTPStatusAndRetries429(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.URL.EscapedPath() != "/v1/storage/object/presign/field-assets/work-orders%2FWO-1042%2Fphoto.jpg" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"expires_seconds":600`) || strings.Contains(string(body), `"bucket"`) {
			t.Fatalf("unexpected body: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"ok":false,"data":null,"error":{"code":"busy","message":"retry later"},"metadata":{}}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"data":{"url":"https://uploads.example/signed"},"error":null,"metadata":{}}`))
	}))
	defer server.Close()

	client := NewClient("test-key")
	client.BaseURL = server.URL
	client.Sleep = func(context.Context, time.Duration) error { return nil }
	got, err := client.PresignPut(context.Background(), "field-assets", "work-orders/WO-1042/photo.jpg", PresignPutInput{
		ContentType: "image/jpeg", MaxBytes: 2048, ExpiresSeconds: 600, IdempotencyKey: "upload-1042",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.URL != "https://uploads.example/signed" || calls != 2 {
		t.Fatalf("result = %+v, calls = %d", got, calls)
	}
}
