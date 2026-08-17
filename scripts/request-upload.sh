#!/bin/sh
set -eu

curl --fail-with-body \
  --request POST \
  --header 'Content-Type: application/json' \
  --data '{"work_order_id":"WO-1042","technician_id":"tech-7","dispatch_status":"en_route","filename":"panel.jpg","content_type":"image/jpeg","size_bytes":2048}' \
  http://localhost:8080/upload-requests
