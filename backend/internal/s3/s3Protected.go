package s3

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"io"
	"log"
	"mime/multipart"
	"strings"
)

var (
	s3ProtectedClient *s3.Client
	uploaderProtected *manager.Uploader
	bucketProtected   string
)

// InitS3Protected initializes the S3 Protected client with the provided configuration
func InitS3Protected(bucketName, accessKey, secretKey, region, endpointURL string) error {
	// Validate required parameters
	if bucketName == "" {
		return fmt.Errorf("bucket name is required")
	}
	if accessKey == "" || secretKey == "" {
		return fmt.Errorf("AWS credentials are required")
	}
	if region == "" {
		return fmt.Errorf("AWS region is required")
	}

	// Create custom endpoint resolver if endpoint URL is provided
	var endpointResolver aws.EndpointResolverWithOptions
	if endpointURL != "" {
		// Ensure the endpoint URL has the https:// prefix
		if !strings.HasPrefix(endpointURL, "http://") && !strings.HasPrefix(endpointURL, "https://") {
			endpointURL = "https://" + endpointURL
		}
		endpointResolver = aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{
				URL:           endpointURL,
				SigningRegion: region,
			}, nil
		})
	}

	// Load AWS configuration with provided credentials and region
	opts := []func(*config.LoadOptions) error{
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
		config.WithRegion(region),
	}

	// Add endpoint resolver if custom endpoint is provided
	if endpointResolver != nil {
		opts = append(opts, config.WithEndpointResolverWithOptions(endpointResolver))
	}

	cfg, err := config.LoadDefaultConfig(context.TODO(), opts...)
	if err != nil {
		return fmt.Errorf("unable to load SDK config: %v", err)
	}

	s3ProtectedClient = s3.NewFromConfig(cfg)
	uploaderProtected = manager.NewUploader(s3ProtectedClient)
	bucketProtected = bucketName

	return nil
}

// UploadFileProtected uploads a file to S3 Protected and returns the file location
// folderPath is optional - if provided, the file will be uploaded to that folder path (e.g., "images/cars/")
func UploadFileProtected(file *multipart.FileHeader, name string, folderPath string) (location string, error error) {
	f, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("failed to open file: %v", err)
	}
	defer func(f multipart.File) {
		err := f.Close()
		if err != nil {
			location = ""
			error = fmt.Errorf("failed to close file: %v", err)
		}
	}(f)

	// Construct the full key (path + filename)
	key := name
	if folderPath != "" {
		// Ensure the folder path ends with a slash
		if !strings.HasSuffix(folderPath, "/") {
			folderPath += "/"
		}
		key = folderPath + name
	}

	// Upload the file to S3
	result, err := uploaderProtected.Upload(context.TODO(), &s3.PutObjectInput{
		Bucket: aws.String(bucketProtected),
		Key:    aws.String(key),
		Body:   f,
	})

	if err != nil {
		return "", fmt.Errorf("failed to upload file: %v", err)
	}

	return result.Location, nil
}

// extractKeyFromURL extracts the object key from a full S3 URL
func extractKeyFromURL(url string) (string, error) {
	if url == "" {
		return "", fmt.Errorf("empty URL provided")
	}

	// Remove the protocol (http:// or https://) if present
	url = strings.TrimPrefix(strings.TrimPrefix(url, "https://"), "http://")

	// Split URL by '/' to separate domain and path
	parts := strings.SplitN(url, "/", 2)
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid URL format: no path found")
	}

	// Return everything after the domain as the key
	return parts[1], nil
}

// DownloadFileProtected downloads a protected file from the protected S3 bucket using either a key or full URL
func DownloadFileProtected(keyOrURL string) ([]byte, error) {
	if keyOrURL == "" {
		return nil, fmt.Errorf("empty input provided")
	}

	// Determine if input is a URL or key
	var key string
	if strings.HasPrefix(keyOrURL, "http") {
		extractedKey, err := extractKeyFromURL(keyOrURL)
		if err != nil {
			return nil, err
		}
		key = extractedKey
	} else {
		key = keyOrURL
	}

	log.Printf("Downloading file from S3 bucket %s with key: %s", bucketProtected, key)

	// Get the object from S3
	result, err := s3ProtectedClient.GetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: aws.String(bucketProtected),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get object from S3: %v", err)
	}
	defer result.Body.Close()

	// Read the object's content
	body, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read object body: %v", err)
	}

	log.Printf("Successfully downloaded %d bytes from S3", len(body))
	return body, nil
}
