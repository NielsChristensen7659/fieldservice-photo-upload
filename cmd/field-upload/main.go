package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	fieldupload "github.com/infrai-examples/fieldservice-photo-upload"
)

func main() {
	apiKey := os.Getenv("INFRAI_API_KEY")
	if apiKey == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	bucket := getenv("ASSET_BUCKET", "fieldservice-assets")
	client := fieldupload.NewClient(apiKey)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := client.EnsureBucket(ctx, bucket); err != nil {
		log.Fatalf("prepare asset bucket: %v", err)
	}

	service := fieldupload.UploadService{Signer: client, Bucket: bucket, Now: time.Now}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /upload-requests", func(w http.ResponseWriter, r *http.Request) {
		var input fieldupload.UploadRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON request"})
			return
		}
		decision, err := service.RequestPhotoUpload(r.Context(), input)
		if err != nil {
			status := http.StatusBadRequest
			var apiErr *fieldupload.APIError
			if errors.As(err, &apiErr) {
				status = apiErr.HTTPStatus
				if status < 400 || status >= 500 {
					status = http.StatusBadGateway
				}
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, decision)
	})

	address := getenv("LISTEN_ADDR", ":8080")
	log.Printf("field upload service listening on %s", address)
	log.Fatal(http.ListenAndServe(address, mux))
}

func getenv(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
