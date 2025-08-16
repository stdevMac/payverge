# Use the official Golang image
FROM golang:1.22.2-alpine AS builder

# Set the working directory
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies
RUN go mod download

# Copy the source from the current directory to the working Directory inside the container
COPY . .

# Build the Go app
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o server ./cmd/app/main.go

# Start a new stage from scratch
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the Pre-built binary file from the previous stage
COPY --from=builder /app/server .

# Create necessary directories
RUN mkdir -p metadata images/nft

# Expose port 8080 to the outside world
EXPOSE 8080

# Define arguments with default values
ARG PASSWORD=password
ARG USERNAME=admin
ARG URI=mongodb://localhost:27017
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

# Command to run the executable
ENTRYPOINT ["./server"]