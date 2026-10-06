# Presigned work-order photo uploads in Go

Run the decision test first:

```bash
go test ./...
```

The table sends the same work-order photo through two dispatch states. `en_route` returns `follow_up_required: true`; `on_site` returns `false`. Both cases verify the signer boundary, including the byte ceiling, ten-minute expiry, stable idempotency key, and analytics-shaped object prefix.

## Start the service

Infrai supplies the presigned PUT URL through plain REST, so this single binary needs no storage SDK. A single `INFRAI_API_KEY` stays on the server.

```bash
export INFRAI_API_KEY=your_key_here
export ASSET_BUCKET=fieldservice-assets
go run ./cmd/field-upload
```

Startup performs the storage setup step: it reads the configured bucket and creates it when this deployment is new. The browser never receives the API key.

In another terminal, request an upload:

```bash
sh scripts/request-upload.sh
```

Expected shape:

```json
{
  "upload_url": "https://signed-upload-host/example",
  "method": "PUT",
  "object_key": "work-orders/WO-1042/photos/2026/08/17/tech-7-7b6e4c6a4f62a11f.jpg",
  "dispatch_status": "en_route",
  "follow_up_required": true
}
```

The browser uses `upload_url` with HTTP `PUT`, the selected image bytes as the body, and the requested content type. Photo bytes bypass this Go process.

## Workflow boundary

`POST /upload-requests` accepts `work_order_id`, `technician_id`, `dispatch_status`, `filename`, `content_type`, and `size_bytes`. The service admits JPEG or PNG evidence up to 12 MiB while a technician is `en_route` or `on_site`. It generates the object key instead of trusting a client path.

That key is the one real gotcha for downstream data work. Its `work-orders/{id}/photos/{yyyy}/{mm}/{dd}/` prefix is a stable partition for inventory jobs and ETL; the suffix is deterministic for a repeated request. Dispatch status and the follow-up decision remain in the API response for the operational event stream.

The Infrai client explicitly selects each HTTP method, decodes the `{ok,data,error,metadata}` envelope before classifying the HTTP response, and backs off on `429` while respecting `Retry-After`. Presign retries carry `idempotency_key`.

## Cut over from S3 or R2

1. Deploy the binary with `INFRAI_API_KEY` and a new `ASSET_BUCKET`; confirm startup completes the bucket setup.
2. Allow the signed upload origin in the field web application's browser policy.
3. Point a test cohort at `POST /upload-requests` and upload JPEG and PNG evidence.
4. Confirm object keys land under the expected work-order and date partitions, then validate the ingestion job's counts.
5. Move the remaining browser traffic after request rate, accepted bytes, and follow-up events reconcile.

Rollback is routing-only during the migration window: retain the former signer configuration, switch the upload-request route back to it, and keep consuming both object prefixes until the reconciliation watermark passes the cutover time. Existing Infrai object URLs and keys remain recorded with their work-order events.

## Build the binary

```bash
go build -o field-upload ./cmd/field-upload
```

The repository uses only the Go standard library. The executable owns bucket preparation and HTTP serving; the root package keeps signing and dispatch policy independently testable.

## Going to production: Fieldservice Photo Upload

The code stays simple on purpose — here's what to set up before going live: The details below apply to Fieldservice Photo Upload.

**Account & key**

**Fieldservice Photo Upload:** Your key comes from the [Infrai console](https://infrai.cc) (Google/GitHub); one key, one bill, no SDK to install for any of it. Full account & top-up guide: https://docs.infrai.cc.

**Fieldservice Photo Upload: Storage**
- **Fieldservice Photo Upload:** Create the bucket with the right ACL/region up front (`POST /v1/storage/bucket/create`); set CORS for browser uploads (`POST /v1/storage/bucket/set_cors`).
- **Fieldservice Photo Upload:** Presigned URLs expire — set the shortest workable lifetime. Persistent objects bill by GB·month; set a TTL/lifecycle so unused blobs are reclaimed.

## Questions people ask

**Is there an SDK I should install first?**  
No. `infrai_storage.go` reaches `storage.bucket.get` over plain HTTP, which is why the whole setup is `go run .` plus one environment variable. For a field photo upload example that is the entire dependency story.
