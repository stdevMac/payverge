# Use the official Golang image with Ubuntu base to avoid Alpine SSL issues
FROM golang:1.22.2-bullseye AS builder

# Install build dependencies for SQLite
RUN apt-get update && apt-get install -y \
    gcc \
    libc6-dev \
    libsqlite3-dev \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Set the working directory
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies
RUN go mod download

# Copy the source from the current directory to the working Directory inside the container
COPY . .

# Build the Go app with CGO enabled for SQLite
RUN CGO_ENABLED=1 GOOS=linux go build -a -o server ./cmd/app/main.go

# Start a new stage from scratch
FROM debian:bullseye-slim

RUN apt-get update && apt-get install -y \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /root/

# Copy the Pre-built binary file from the previous stage
COPY --from=builder /app/server .

# Create necessary directories
RUN mkdir -p metadata images/nft data

# Expose port 8080 to the outside world
EXPOSE 8080

# Define arguments with default values
ARG DATABASE_PATH=./data/app.db
ARG AWS_ACCESS_KEY=aws_access_key
ARG AWS_SECRET_KEY=aws_secret_key
ARG AWS_REGION=us-east-1
ARG S3_BUCKET=bucket_name
ARG S3_ENDPOINT=s3_endpoint
ARG S3_PROTECTED_BUCKET=s3_protected_bucket
ARG AWS_PROTECTED_ACCESS_KEY=aws_protected_access_key
ARG AWS_PROTECTED_SECRET_KEY=aws_protected_secret_key
ARG S3_PROTECTED_ENDPOINT=s3_protected_endpoint
ARG FAUCET_PRIVATE_KEY=faucet_private_key
ARG RPC_URL=rpc_url
ARG CHAIN_ID=chain_id
ARG USDC_CONTRACT=usdc_contract
ARG PAYVERGE_CONTRACT=payverge_contract
ARG TELEGRAM_TOKEN=telegram_token
ARG POSTMARK_TOKEN=postmark_token
ARG POSTMARK_FROM_TRANSACTIONAL=postmark_from_transactional
ARG POSTMARK_FROM_NEWS=postmark_from_news
ARG POSTMARK_FROM_UPDATES=postmark_from_updates

# Command to run the executable
ENTRYPOINT ["./server"]