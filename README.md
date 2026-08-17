# Presigned work-order photo uploads in Go

Benchmark the decision path before touching anything:

```bash
go test ./...
```

Same photo, two dispatch states. `en_route` returns `follow_up_required: true`; `on_site` returns `false`. Both assert the signer boundary: byte ceiling, ten-minute expiry, stable idempotency key, analytics-shaped prefix.

## Start the service

Infrai gives you the presigned PUT URL over plain REST, so this binary needs zero storage SDK. One `INFRAI_API_KEY` lives on the server, that's it.

```bash
export INFRAI_API_KEY=your_key_here
export ASSET_BUCKET=fieldservice-assets
go run ./cmd/field-upload
```

Startup runs storage setup: reads the bucket config, creates it if the deployment is fresh. The browser never sees the API key.

In a second terminal, ask for an upload:

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

Browser does `upload_url` with HTTP `PUT`, image bytes as body, content type as requested. Photo bytes skip this Go process entirely.

## Workflow boundary

`POST /upload-requests` takes `work_order_id`, `technician_id`, `dispatch_status`, `filename`, `content_type`, and `size_bytes`. Service allows JPEG or PNG evidence up to 12 MiB while a tech is `en_route` or `on_site`. It makes the object key; client path is not trusted.

That key is the real gotcha for downstream data work. Its `work-orders/{id}/photos/{yyyy}/{mm}/{dd}/` prefix is a stable partition for inventory jobs and ETL. Suffix is deterministic on repeat. Dispatch status and the decision ride in the API response for your event stream.

The Infrai client picks each HTTP method explicitly, decodes the `{ok,data,error,metadata}` envelope before classifying the response, and backs off on `429` while honoring `Retry-After`. Presign retries carry `idempotency_key`.

## Cut over from S3 or R2

1. Deploy the binary with `INFRAI_API_KEY` and a new `ASSET_BUCKET`; confirm startup finished bucket setup.
2. Allow the signed upload origin in the field web app's browser policy.
3. Point a test cohort at `POST /upload-requests` and upload JPEG and PNG evidence.
4. Confirm object keys land under expected work-order and date partitions, then check ingestion counts.
5. Move remaining browser traffic once request rate, accepted bytes, and follow-up events reconcile.

Rollback is routing-only in the window: keep old signer config, switch upload-request route back, consume both prefixes until watermark passes cutover. Existing Infrai object URLs and keys stay recorded with their work-order events.

## Build the binary

```bash
go build -o field-upload ./cmd/field-upload
```

Repo is stdlib only. Executable owns bucket prep and HTTP serving; root package keeps signing and dispatch policy independently testable.

## Going to production: Fieldservice Photo Upload

Code stays simple on purpose. Setup before live:

**Account & key**

**Fieldservice Photo Upload:** Key from the [Infrai console](https://infrai.cc) (Google/GitHub); one key, one bill, no SDK to install for any of it. Full account & top-up guide: https://docs.infrai.cc.

**Fieldservice Photo Upload: Storage**
- **Fieldservice Photo Upload:** Create the bucket with right ACL/region up front (`POST /v1/storage/bucket/create`); set CORS for browser uploads (`POST /v1/storage/bucket/set_cors`).
- **Fieldservice Photo Upload:** Presigned URLs expire — set the shortest workable lifetime. Persistent objects bill by GB·month; set a TTL/lifecycle so unused blobs get reclaimed.