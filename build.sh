#!/bin/bash

hash=$(git rev-parse --short HEAD)
GOOS=windows GOARCH=amd64 go build -ldflags "-X main.Hash=$hash" -o dist/mt-to-exante-sdk.exe cmd/api/main.go
