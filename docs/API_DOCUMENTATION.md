# API Documentation

## Overview

This document provides comprehensive documentation for the Web3 Boilerplate API endpoints. The API is built with Go using the Gin framework and provides authentication, user management, faucet functionality, file uploads, and administrative features.

## Base URL

```
http://localhost:8080/api/v1
```

## Authentication

The API uses SIWE (Sign-In with Ethereum) for authentication. Most protected endpoints require a valid JWT token in the Authorization header.

### Headers

```
Authorization: Bearer <jwt_token>
Content-Type: application/json
```

## Rate Limiting

- **General endpoints**: 100 requests per minute per IP
- **Auth endpoints**: 10 requests per minute per IP
- **File upload**: 5 requests per minute per IP

## Error Responses

All endpoints return consistent error responses:

```json
{
  "error": "Error message description"
}
```

Common HTTP status codes:
- `400` - Bad Request
- `401` - Unauthorized
- `403` - Forbidden
- `404` - Not Found
- `429` - Too Many Requests
- `500` - Internal Server Error

---

## Authentication Endpoints

### Generate Challenge

Generate a challenge for SIWE authentication.

**Endpoint:** `POST /auth/challenge`

**Request Body:**
```json
{
  "address": "0x742d35Cc6634C0532925a3b8D0c6E0b4C2c8c8c8"
}
```

**Response:**
```json
{
  "challenge": "unique-challenge-string"
}
```

**Status Codes:**
- `200` - Success
- `400` - Invalid address format
- `500` - Server error

---

### Sign In

Authenticate using SIWE message and signature.

**Endpoint:** `POST /auth/signin`

**Request Body:**
```json
{
  "message": "localhost:3000 wants you to sign in with your Ethereum account:\n0x742d35Cc6634C0532925a3b8D0c6E0b4C2c8c8c8\n\nSign in with Ethereum to the app.\n\nURI: http://localhost:3000\nVersion: 1\nChain ID: 1\nNonce: challenge-string\nIssued At: 2023-01-01T00:00:00.000Z",
  "signature": "0x..."
}
```

**Response:**
```json
{
  "token": "jwt-token-string",
  "user": {
    "address": "0x742d35Cc6634C0532925a3b8D0c6E0b4C2c8c8c8",
    "role": "user",
    "created_at": "2023-01-01T00:00:00Z"
  }
}
```

**Status Codes:**
- `200` - Success
- `400` - Invalid message or signature
- `401` - Authentication failed
- `500` - Server error

---

### Get Session

Retrieve current user session information.

**Endpoint:** `GET /auth/session`

**Headers:** `Authorization: Bearer <token>`

**Response:**
```json
{
  "user": {
    "address": "0x742d35Cc6634C0532925a3b8D0c6E0b4C2c8c8c8",
    "role": "user",
    "created_at": "2023-01-01T00:00:00Z"
  }
}
```

**Status Codes:**
- `200` - Success
- `401` - Invalid or expired token

---

### Sign Out

Invalidate current session.

**Endpoint:** `POST /auth/signout`

**Headers:** `Authorization: Bearer <token>`

**Response:**
```json
{
  "message": "Signed out successfully"
}
```

**Status Codes:**
- `200` - Success
- `401` - Invalid token

---

## Public Endpoints

### Health Check

Check API health status.

**Endpoint:** `GET /health`

**Response:**
```json
{
  "status": "healthy",
  "timestamp": "2023-01-01T00:00:00Z"
}
```

**Status Codes:**
- `200` - Service healthy

---

### Telegram Webhook

Webhook endpoint for Telegram bot integration.

**Endpoint:** `POST /telegram/webhook`

**Request Body:** Telegram webhook payload

**Response:**
```json
{
  "status": "processed"
}
```

**Status Codes:**
- `200` - Webhook processed
- `400` - Invalid payload

---

### Subscribe

Subscribe to notifications.

**Endpoint:** `POST /subscribe`

**Request Body:**
```json
{
  "email": "user@example.com",
  "telegram_id": "123456789"
}
```

**Response:**
```json
{
  "message": "Subscribed successfully",
  "subscriber_id": "unique-id"
}
```

**Status Codes:**
- `200` - Success
- `400` - Invalid email or telegram ID
- `409` - Already subscribed

---

### Unsubscribe

Unsubscribe from notifications.

**Endpoint:** `POST /unsubscribe`

**Request Body:**
```json
{
  "email": "user@example.com"
}
```

**Response:**
```json
{
  "message": "Unsubscribed successfully"
}
```

**Status Codes:**
- `200` - Success
- `404` - Subscriber not found

---

### Log Error

Log client-side errors.

**Endpoint:** `POST /log-error`

**Request Body:**
```json
{
  "error": "Error message",
  "stack": "Stack trace",
  "url": "https://example.com/page",
  "user_agent": "Browser info"
}
```

**Response:**
```json
{
  "message": "Error logged"
}
```

**Status Codes:**
- `200` - Success

---

## Protected Endpoints

*Requires authentication*

### Get User Profile

Retrieve current user profile.

**Endpoint:** `GET /user`

**Headers:** `Authorization: Bearer <token>`

**Response:**
```json
{
  "address": "0x742d35Cc6634C0532925a3b8D0c6E0b4C2c8c8c8",
  "role": "user",
  "created_at": "2023-01-01T00:00:00Z",
  "settings": {
    "notifications": true,
    "language": "en"
  }
}
```

**Status Codes:**
- `200` - Success
- `401` - Unauthorized

---

### Update User Settings

Update user preferences and settings.

**Endpoint:** `PUT /user/settings`

**Headers:** `Authorization: Bearer <token>`

**Request Body:**
```json
{
  "notifications": true,
  "language": "en"
}
```

**Response:**
```json
{
  "message": "Settings updated successfully"
}
```

**Status Codes:**
- `200` - Success
- `400` - Invalid settings
- `401` - Unauthorized

---

### Faucet Top-up

Request ETH from faucet (testnet only).

**Endpoint:** `POST /faucet/topup`

**Headers:** `Authorization: Bearer <token>`

**Request Body:**
```json
{
  "address": "0x742d35Cc6634C0532925a3b8D0c6E0b4C2c8c8c8",
  "amount": "0.1"
}
```

**Response:**
```json
{
  "transaction_hash": "0x...",
  "amount": "0.1",
  "status": "pending"
}
```

**Status Codes:**
- `200` - Success
- `400` - Invalid address or amount
- `401` - Unauthorized
- `429` - Rate limit exceeded

---

### Upload File

Upload file to S3 storage.

**Endpoint:** `POST /upload`

**Headers:** 
- `Authorization: Bearer <token>`
- `Content-Type: multipart/form-data`

**Request Body:** Form data with file

**Response:**
```json
{
  "file_url": "https://s3.amazonaws.com/bucket/file.jpg",
  "file_id": "unique-file-id"
}
```

**Status Codes:**
- `200` - Success
- `400` - Invalid file
- `401` - Unauthorized
- `413` - File too large

---

## Admin Endpoints

*Requires admin role*

### Get All Users

Retrieve list of all users.

**Endpoint:** `GET /admin/users`

**Headers:** `Authorization: Bearer <admin_token>`

**Query Parameters:**
- `page` (optional): Page number (default: 1)
- `limit` (optional): Items per page (default: 20)

**Response:**
```json
{
  "users": [
    {
      "address": "0x742d35Cc6634C0532925a3b8D0c6E0b4C2c8c8c8",
      "role": "user",
      "created_at": "2023-01-01T00:00:00Z"
    }
  ],
  "total": 100,
  "page": 1,
  "limit": 20
}
```

**Status Codes:**
- `200` - Success
- `401` - Unauthorized
- `403` - Forbidden (not admin)

---

### Update User Role

Update user role (admin only).

**Endpoint:** `PUT /admin/users/:address/role`

**Headers:** `Authorization: Bearer <admin_token>`

**Request Body:**
```json
{
  "role": "admin"
}
```

**Response:**
```json
{
  "message": "User role updated successfully"
}
```

**Status Codes:**
- `200` - Success
- `400` - Invalid role
- `401` - Unauthorized
- `403` - Forbidden
- `404` - User not found

---

### Get Subscribers

Retrieve list of subscribers.

**Endpoint:** `GET /admin/subscribers`

**Headers:** `Authorization: Bearer <admin_token>`

**Response:**
```json
{
  "subscribers": [
    {
      "id": "unique-id",
      "email": "user@example.com",
      "telegram_id": "123456789",
      "created_at": "2023-01-01T00:00:00Z"
    }
  ]
}
```

**Status Codes:**
- `200` - Success
- `401` - Unauthorized
- `403` - Forbidden

---

### Multisig Transactions

Get multisig transaction history.

**Endpoint:** `GET /admin/multisig/transactions`

**Headers:** `Authorization: Bearer <admin_token>`

**Response:**
```json
{
  "transactions": [
    {
      "id": "tx-id",
      "to": "0x...",
      "value": "1000000000000000000",
      "data": "0x...",
      "status": "pending",
      "confirmations": 1,
      "required_confirmations": 2,
      "created_at": "2023-01-01T00:00:00Z"
    }
  ]
}
```

**Status Codes:**
- `200` - Success
- `401` - Unauthorized
- `403` - Forbidden

---

### Generate Codes

Generate invitation or access codes.

**Endpoint:** `POST /admin/codes/generate`

**Headers:** `Authorization: Bearer <admin_token>`

**Request Body:**
```json
{
  "type": "invitation",
  "count": 10,
  "expires_at": "2023-12-31T23:59:59Z"
}
```

**Response:**
```json
{
  "codes": [
    {
      "code": "ABC123DEF",
      "type": "invitation",
      "expires_at": "2023-12-31T23:59:59Z"
    }
  ]
}
```

**Status Codes:**
- `200` - Success
- `400` - Invalid parameters
- `401` - Unauthorized
- `403` - Forbidden

---

### Get Codes

Retrieve generated codes.

**Endpoint:** `GET /admin/codes`

**Headers:** `Authorization: Bearer <admin_token>`

**Response:**
```json
{
  "codes": [
    {
      "code": "ABC123DEF",
      "type": "invitation",
      "used": false,
      "expires_at": "2023-12-31T23:59:59Z",
      "created_at": "2023-01-01T00:00:00Z"
    }
  ]
}
```

**Status Codes:**
- `200` - Success
- `401` - Unauthorized
- `403` - Forbidden

---

## WebSocket Endpoints

### Real-time Notifications

Connect to real-time notification stream.

**Endpoint:** `WS /ws/notifications`

**Headers:** `Authorization: Bearer <token>`

**Message Format:**
```json
{
  "type": "notification",
  "data": {
    "message": "Transaction confirmed",
    "transaction_hash": "0x..."
  }
}
```

---

## Security Features

### Input Validation

All endpoints implement comprehensive input validation:
- SQL injection protection
- XSS prevention
- Command injection detection
- Path traversal protection

### Rate Limiting

Different rate limits apply based on endpoint sensitivity:
- Authentication: 10 req/min
- File uploads: 5 req/min
- General API: 100 req/min

### CORS

Configured for cross-origin requests with appropriate headers.

### Security Headers

- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `X-XSS-Protection: 1; mode=block`
- `Referrer-Policy: strict-origin-when-cross-origin`

---

## Environment Configuration

Required environment variables:

```bash
# Database
MONGODB_URI=mongodb://localhost:27017/web3boilerplate

# Ethereum
ETH_RPC_URL=https://mainnet.infura.io/v3/your-key
ETH_PRIVATE_KEY=your-private-key

# JWT
JWT_SECRET=your-jwt-secret

# S3 Storage
AWS_ACCESS_KEY_ID=your-access-key
AWS_SECRET_ACCESS_KEY=your-secret-key
AWS_REGION=us-east-1
AWS_BUCKET_NAME=your-bucket

# External Services
POSTHOG_API_KEY=your-posthog-key
TELEGRAM_BOT_TOKEN=your-telegram-token
SENDGRID_API_KEY=your-sendgrid-key
```

---

## Testing

Run API tests:

```bash
cd backend
make test
```

Test coverage:
```bash
make test-coverage
```

---

## Monitoring

The API includes Prometheus metrics at `/metrics` endpoint for monitoring:
- Request duration
- Request count by endpoint
- Error rates
- Active connections
