#!/bin/bash

# Script to generate test TLS certificates for redhub tests

set -e

cd "$(dirname "$0")"

# Generate private key
openssl genrsa -out key.pem 2048

# Generate certificate
openssl req -new -x509 -key key.pem -out cert.pem -days 365 \
  -subj "/C=US/ST=California/L=San Francisco/O=RedHub/CN=localhost"

echo "Test certificates generated successfully!"
