#!/bin/bash
set -e

echo "=== Creating multipart split archive ==="
mkdir -p data/manager/test_source

# Create a larger file (approx 10KB) to guarantee multiple 1KB volumes
dd if=/dev/urandom of=data/manager/test_source/hello.bin bs=1024 count=10

# Create multi-part archive split into 1KB volumes
rm -f data/manager/multipart_archive.7z.*
7z a -v1k data/manager/multipart_archive.7z data/manager/test_source/hello.bin

echo "Created files:"
ls -lh data/manager/multipart_archive.7z.*

# Authenticate to get JWT token
echo "=== Authenticating ==="
token=$(curl -s -X POST http://127.0.0.1:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"salman","password":"136517"}' | grep -oP '"token":"\K[^"]+')

echo "Token successfully acquired: ${token:0:10}..."

# Decompress by pointing to the second part (multipart_archive.7z.002) to verify auto-redirection to the first part
echo "=== Requesting decompress on multipart_archive.7z.002 ==="
curl -s -X POST http://127.0.0.1:8080/api/files/decompress \
  -H "Authorization: Bearer $token" \
  -H "Content-Type: application/json" \
  -d '{"path":"multipart_archive.7z.002"}'

echo ""
echo "=== Waiting for decompress job ==="
sleep 2

# Check extracted files
echo "=== Listing extracted files ==="
if [ -f "data/manager/multipart_archive/test_source/hello.bin" ]; then
  echo "SUCCESS: Found hello.bin in multipart_archive folder!"
  ls -lh "data/manager/multipart_archive/test_source/hello.bin"
else
  echo "FAILURE: hello.bin not found!"
  exit 1
fi
