const fs = require('fs').promises;
const path = require('path');
const { S3Client, PutObjectCommand } = require('@aws-sdk/client-s3');
require('dotenv').config();

// Set environment variables
process.env.NEXT_PUBLIC_BUCKET_URL = 'https://j5f7.c18.e2-3.dev/shares';
process.env.NEXT_PUBLIC_AWS_ACCESS_KEY_ID = 'c8w4VBnEKKpVUAHj9s2M';
process.env.NEXT_PUBLIC_AWS_SECRET_ACCESS_KEY = 'QFup9USpPQ5xnuLPcEB3594SXV1dBAFRLFYjlE5A';

// Initialize S3 Client
const s3Client = new S3Client({
    endpoint: process.env.NEXT_PUBLIC_BUCKET_URL,
    region: "eu-central-1",
    credentials: {
        accessKeyId: process.env.NEXT_PUBLIC_AWS_ACCESS_KEY_ID,
        secretAccessKey: process.env.NEXT_PUBLIC_AWS_SECRET_ACCESS_KEY,
    },
    forcePathStyle: true,
});

async function uploadFileToS3(filePath, name, folder) {
    const fileContent = await fs.readFile(filePath);
    const fileName = path.basename(filePath);

    const params = {
        Bucket: "shares",
        Key: `${folder}/${name}-${Date.now()}-${fileName}`,
        Body: fileContent,
        ACL: "public-read",
    };

    try {
        const command = new PutObjectCommand(params);
        await s3Client.send(command);

        const location = `${process.env.NEXT_PUBLIC_BUCKET_URL}/${params.Bucket}/${params.Key}`;
        console.log("Uploaded:", fileName);
        console.log("URL:", location);
        return location;
    } catch (error) {
        console.error(`Error uploading ${fileName}:`, error);
        throw error;
    }
}

const getRandomName = () => {
    return Math.random().toString(36).slice(2, 10);
};

async function uploadFolderContents(folderPath) {
    try {
        // Read all files in the directory
        const files = await fs.readdir(folderPath);
        const imageFiles = files.filter(file => 
            /\.(jpg|jpeg|png|gif|webp|avif)$/i.test(file)
        );

        if (imageFiles.length === 0) {
            console.log('No image files found in the specified folder');
            return;
        }

        console.log(`Found ${imageFiles.length} image files. Starting upload...`);
        
        const name = getRandomName();
        const uploadPromises = imageFiles.map(file => 
            uploadFileToS3(path.join(folderPath, file), name, "random_images")
        );

        const urls = await Promise.all(uploadPromises);
        
        console.log('\nAll uploads completed!');
        console.log('\nUploaded URLs:');
        urls.forEach(url => console.log("\""+url+"\","));
        
    } catch (error) {
        console.error('Error processing folder:', error);
    }
}

// Get folder path from command line argument
const folderPath = process.argv[2];

if (!folderPath) {
    console.error('Please provide a folder path as an argument');
    console.log('Usage: node batchUploadToS3.js <folder-path>');
    process.exit(1);
}

uploadFolderContents(folderPath);
