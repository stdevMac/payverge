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
	"net/http"
	"strings"
	"time"
)

var (
	s3Client *s3.Client
	uploader *manager.Uploader
	bucket   string
)

// InitS3 initializes the S3 client with the provided configuration
func InitS3(bucketName, accessKey, secretKey, region, endpointURL string) error {
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

	s3Client = s3.NewFromConfig(cfg)
	uploader = manager.NewUploader(s3Client)
	bucket = bucketName

	return nil
}

// UploadFile uploads a file to S3 and returns the file location
// folderPath is optional - if provided, the file will be uploaded to that folder path (e.g., "images/cars/")
func UploadFile(file *multipart.FileHeader, name string, folderPath string) (location string, error error) {
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
	result, err := uploader.Upload(context.TODO(), &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   f,
	})

	if err != nil {
		return "", fmt.Errorf("failed to upload file: %v", err)
	}

	return result.Location, nil
}

// DownloadFile downloads a file from a URL and returns its contents
func DownloadFile(url string) ([]byte, error) {
	if url == "" {
		return nil, fmt.Errorf("empty URL provided")
	}

	log.Printf("Downloading file from URL: %s", url)

	// Create an HTTP client
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Make the request
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to download file: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download file: status code %d", resp.StatusCode)
	}

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	log.Printf("Successfully downloaded %d bytes", len(body))
	return body, nil
}
