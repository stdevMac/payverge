# Usage and Implementation Guide

## Overview

This guide provides comprehensive instructions for setting up, configuring, and using the Web3 Boilerplate project. It covers development setup, deployment, customization, and best practices.

## Table of Contents

1. [Quick Start](#quick-start)
2. [Development Setup](#development-setup)
3. [Environment Configuration](#environment-configuration)
4. [Running the Application](#running-the-application)
5. [Testing](#testing)
6. [Deployment](#deployment)
7. [Customization Guide](#customization-guide)
8. [API Usage Examples](#api-usage-examples)
9. [Frontend Integration](#frontend-integration)
10. [Smart Contract Development](#smart-contract-development)
11. [Troubleshooting](#troubleshooting)
12. [Best Practices](#best-practices)

---

## Quick Start

### Prerequisites

- **Node.js** 18+ and npm/yarn
- **Go** 1.21+
- **Docker** and Docker Compose
- **Git** with submodules support
- **Foundry** for smart contracts

### 1-Minute Setup

```bash
# Clone the repository
git clone --recursive https://github.com/your-org/web3-boilerplate.git
cd web3-boilerplate

# Copy environment files
cp .env.example .env
cp frontend/.env.development.sample frontend/.env.development
cp backend/.env.example backend/.env

# Start all services
./start.sh
```

The application will be available at:
- **Frontend**: http://localhost:3000
- **Backend API**: http://localhost:8080
- **MongoDB**: localhost:27017

---

## Development Setup

### Manual Setup

#### 1. Clone Repository

```bash
git clone --recursive https://github.com/your-org/web3-boilerplate.git
cd web3-boilerplate

# If you forgot --recursive
git submodule update --init --recursive
```

#### 2. Backend Setup

```bash
cd backend

# Install dependencies
go mod download

# Run database migrations (if any)
go run cmd/app/main.go migrate

# Start development server
make dev
```

#### 3. Frontend Setup

```bash
cd frontend

# Install dependencies
npm install

# Start development server
npm run dev
```

#### 4. Smart Contracts Setup

```bash
cd contracts

# Install Foundry (if not installed)
curl -L https://foundry.paradigm.xyz | bash
foundryup

# Install dependencies
forge install

# Run tests
forge test

# Deploy locally
anvil # In separate terminal
forge script script/Counter.s.sol --rpc-url http://localhost:8545 --private-key 0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80 --broadcast
```

### Docker Setup

```bash
# Start all services
docker-compose up -d

# View logs
docker-compose logs -f

# Stop services
docker-compose down
```

---

## Environment Configuration

### Backend Environment Variables

Create `backend/.env`:

```bash
# Database
MONGODB_URI=mongodb://localhost:27017/web3boilerplate
MONGODB_DATABASE=web3boilerplate

# Server
PORT=8080
GIN_MODE=release

# JWT Authentication
JWT_SECRET=your-super-secret-jwt-key-change-this-in-production

# Ethereum Configuration
ETH_RPC_URL=https://mainnet.infura.io/v3/your-infura-key
ETH_PRIVATE_KEY=your-private-key-for-faucet
ETH_CHAIN_ID=1

# AWS S3 Storage
AWS_ACCESS_KEY_ID=your-aws-access-key
AWS_SECRET_ACCESS_KEY=your-aws-secret-key
AWS_REGION=us-east-1
AWS_BUCKET_NAME=your-s3-bucket

# External Services
POSTHOG_API_KEY=your-posthog-key
TELEGRAM_BOT_TOKEN=your-telegram-bot-token
SENDGRID_API_KEY=your-sendgrid-api-key

# Development/Testing
DISABLE_POSTHOG=true
DISABLE_TELEGRAM=true
```

### Frontend Environment Variables

Create `frontend/.env.development`:

```bash
# API Configuration
NEXT_PUBLIC_API_URL=http://localhost:8080/api/v1
NEXT_PUBLIC_WS_URL=ws://localhost:8080/ws

# Web3 Configuration
NEXT_PUBLIC_WALLETCONNECT_PROJECT_ID=your-walletconnect-project-id
NEXT_PUBLIC_CHAIN_ID=1

# Feature Flags
NEXT_PUBLIC_MAINTENANCE_MODE=false
NEXT_PUBLIC_COMING_SOON_MODE=false

# Analytics
NEXT_PUBLIC_POSTHOG_KEY=your-posthog-key
NEXT_PUBLIC_POSTHOG_HOST=https://app.posthog.com
```

---

## Running the Application

### Development Mode

#### Start All Services

```bash
# Using the convenience script
./start.sh

# Or manually start each service
cd backend && make dev &
cd frontend && npm run dev &
cd contracts && anvil &
```

#### Individual Services

```bash
# Backend only
cd backend
make dev

# Frontend only
cd frontend
npm run dev

# Local blockchain
cd contracts
anvil
```

### Production Mode

```bash
# Build all services
cd backend && make build
cd frontend && npm run build

# Start with Docker Compose
docker-compose -f docker-compose.prod.yml up -d
```

---

## Testing

### Backend Tests

```bash
cd backend

# Run all tests
make test

# Run with coverage
make test-coverage

# Run specific test
go test ./internal/server -v

# Run integration tests
make test-integration
```

### Frontend Tests

```bash
cd frontend

# Run unit tests
npm run test

# Run with coverage
npm run test:coverage

# Run E2E tests
npm run test:e2e
```

### Smart Contract Tests

```bash
cd contracts

# Run all tests
forge test

# Run with verbosity
forge test -vvv

# Run specific test
forge test --match-test testIncrement

# Generate coverage report
forge coverage
```

---

## Deployment

### Backend Deployment

#### Docker Deployment

```bash
cd backend

# Build image
docker build -t web3-boilerplate-backend .

# Run container
docker run -p 8080:8080 --env-file .env web3-boilerplate-backend
```

#### Manual Deployment

```bash
cd backend

# Build binary
make build

# Run binary
./bin/app
```

### Frontend Deployment

#### Vercel (Recommended)

```bash
# Install Vercel CLI
npm i -g vercel

cd frontend

# Deploy
vercel --prod
```

#### Docker Deployment

```bash
cd frontend

# Build image
docker build -t web3-boilerplate-frontend .

# Run container
docker run -p 3000:3000 web3-boilerplate-frontend
```

### Smart Contract Deployment

#### Mainnet Deployment

```bash
cd contracts

# Deploy to mainnet
forge script script/Counter.s.sol \
  --rpc-url $ETH_RPC_URL \
  --private-key $PRIVATE_KEY \
  --broadcast \
  --verify \
  --etherscan-api-key $ETHERSCAN_API_KEY
```

#### Testnet Deployment

```bash
# Deploy to Sepolia
forge script script/Counter.s.sol \
  --rpc-url https://sepolia.infura.io/v3/your-key \
  --private-key $PRIVATE_KEY \
  --broadcast
```

---

## Customization Guide

### Adding New API Endpoints

#### 1. Create Handler

```go
// backend/internal/server/new_handlers.go
package server

import (
    "net/http"
    "github.com/gin-gonic/gin"
)

func (s *Server) HandleNewFeature(c *gin.Context) {
    // Implementation
    c.JSON(http.StatusOK, gin.H{"message": "success"})
}
```

#### 2. Register Route

```go
// backend/cmd/app/main.go
protected := r.Group("/api/v1/protected")
protected.Use(middleware.AuthRequired())
{
    protected.POST("/new-feature", server.HandleNewFeature)
}
```

#### 3. Add Tests

```go
// backend/internal/server/new_handlers_test.go
func TestHandleNewFeature(t *testing.T) {
    // Test implementation
}
```

### Adding Frontend Components

#### 1. Create Component

```tsx
// frontend/src/components/features/NewFeature.tsx
import React from 'react';

interface NewFeatureProps {
  data: string;
}

export const NewFeature: React.FC<NewFeatureProps> = ({ data }) => {
  return (
    <div className="p-4">
      <h2>New Feature</h2>
      <p>{data}</p>
    </div>
  );
};
```

#### 2. Add API Integration

```typescript
// frontend/src/api/newFeature.ts
import { apiClient } from './client';

export const newFeatureApi = {
  getData: async () => {
    const response = await apiClient.get('/new-feature');
    return response.data;
  },
};
```

#### 3. Create Hook

```typescript
// frontend/src/hooks/useNewFeature.ts
import { useState, useEffect } from 'react';
import { newFeatureApi } from '@/api/newFeature';

export const useNewFeature = () => {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const fetchData = async () => {
      try {
        const result = await newFeatureApi.getData();
        setData(result);
      } catch (error) {
        console.error('Error fetching data:', error);
      } finally {
        setLoading(false);
      }
    };

    fetchData();
  }, []);

  return { data, loading };
};
```

### Adding Smart Contracts

#### 1. Create Contract

```solidity
// contracts/src/NewContract.sol
// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.13;

contract NewContract {
    uint256 public value;

    function setValue(uint256 _value) public {
        value = _value;
    }
}
```

#### 2. Create Test

```solidity
// contracts/test/NewContract.t.sol
// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.13;

import {Test} from "forge-std/Test.sol";
import {NewContract} from "../src/NewContract.sol";

contract NewContractTest is Test {
    NewContract public newContract;

    function setUp() public {
        newContract = new NewContract();
    }

    function testSetValue() public {
        newContract.setValue(42);
        assertEq(newContract.value(), 42);
    }
}
```

#### 3. Create Deployment Script

```solidity
// contracts/script/NewContract.s.sol
// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.13;

import {Script} from "forge-std/Script.sol";
import {NewContract} from "../src/NewContract.sol";

contract NewContractScript is Script {
    function run() public {
        vm.startBroadcast();
        new NewContract();
        vm.stopBroadcast();
    }
}
```

---

## API Usage Examples

### Authentication Flow

```typescript
// 1. Generate challenge
const challengeResponse = await fetch('/api/v1/auth/challenge', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ address: userAddress })
});
const { challenge } = await challengeResponse.json();

// 2. Sign message with wallet
const message = createSiweMessage({
  domain: window.location.host,
  address: userAddress,
  statement: 'Sign in with Ethereum to the app.',
  uri: window.location.origin,
  version: '1',
  chainId: 1,
  nonce: challenge,
  issuedAt: new Date().toISOString(),
});

const signature = await signMessage({ message });

// 3. Sign in
const signInResponse = await fetch('/api/v1/auth/signin', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ message, signature })
});
const { token } = await signInResponse.json();

// 4. Use token for authenticated requests
const userResponse = await fetch('/api/v1/user', {
  headers: { 'Authorization': `Bearer ${token}` }
});
```

### File Upload

```typescript
const uploadFile = async (file: File) => {
  const formData = new FormData();
  formData.append('file', file);

  const response = await fetch('/api/v1/upload', {
    method: 'POST',
    headers: { 'Authorization': `Bearer ${token}` },
    body: formData
  });

  const { file_url } = await response.json();
  return file_url;
};
```

### Faucet Usage

```typescript
const requestFaucet = async (address: string, amount: string) => {
  const response = await fetch('/api/v1/faucet/topup', {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${token}`,
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({ address, amount })
  });

  const { transaction_hash } = await response.json();
  return transaction_hash;
};
```

---

## Frontend Integration

### Web3 Setup

```typescript
// src/lib/web3.ts
import { createAppKit } from '@reown/appkit/react'
import { WagmiProvider } from 'wagmi'
import { arbitrum, mainnet } from '@reown/appkit/networks'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { WagmiAdapter } from '@reown/appkit-adapter-wagmi'

const queryClient = new QueryClient()

const projectId = process.env.NEXT_PUBLIC_WALLETCONNECT_PROJECT_ID!

const metadata = {
  name: 'Web3 Boilerplate',
  description: 'Web3 Boilerplate Application',
  url: 'https://web3boilerplate.com',
  icons: ['https://web3boilerplate.com/icon.png']
}

const networks = [mainnet, arbitrum]

const wagmiAdapter = new WagmiAdapter({
  networks,
  projectId,
  ssr: true
})

createAppKit({
  adapters: [wagmiAdapter],
  networks,
  projectId,
  metadata,
  features: {
    analytics: true
  }
})

export function Web3Provider({ children }: { children: React.ReactNode }) {
  return (
    <WagmiProvider config={wagmiAdapter.wagmiConfig}>
      <QueryClientProvider client={queryClient}>
        {children}
      </QueryClientProvider>
    </WagmiProvider>
  )
}
```

### Authentication Hook

```typescript
// src/hooks/useAuth.ts
import { useAccount, useSignMessage } from 'wagmi'
import { useState } from 'react'
import { createSiweMessage } from 'viem/siwe'

export const useAuth = () => {
  const { address } = useAccount()
  const { signMessageAsync } = useSignMessage()
  const [isLoading, setIsLoading] = useState(false)

  const signIn = async () => {
    if (!address) return

    setIsLoading(true)
    try {
      // Generate challenge
      const challengeRes = await fetch('/api/v1/auth/challenge', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ address })
      })
      const { challenge } = await challengeRes.json()

      // Create SIWE message
      const message = createSiweMessage({
        domain: window.location.host,
        address,
        statement: 'Sign in with Ethereum to the app.',
        uri: window.location.origin,
        version: '1',
        chainId: 1,
        nonce: challenge,
        issuedAt: new Date().toISOString(),
      })

      // Sign message
      const signature = await signMessageAsync({ message })

      // Sign in
      const signInRes = await fetch('/api/v1/auth/signin', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ message, signature })
      })

      const { token } = await signInRes.json()
      localStorage.setItem('auth_token', token)
      
      return token
    } finally {
      setIsLoading(false)
    }
  }

  return { signIn, isLoading }
}
```

---

## Smart Contract Development

### Development Workflow

```bash
# 1. Write contract
vim src/MyContract.sol

# 2. Write tests
vim test/MyContract.t.sol

# 3. Run tests
forge test

# 4. Deploy locally
anvil # separate terminal
forge script script/MyContract.s.sol --rpc-url http://localhost:8545 --private-key 0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80 --broadcast

# 5. Verify deployment
cast call <contract_address> "myFunction()" --rpc-url http://localhost:8545
```

### Integration with Frontend

```typescript
// src/hooks/useContract.ts
import { useReadContract, useWriteContract } from 'wagmi'
import { contractAbi } from '@/contracts/MyContract'

const CONTRACT_ADDRESS = '0x...'

export const useMyContract = () => {
  const { data: value } = useReadContract({
    address: CONTRACT_ADDRESS,
    abi: contractAbi,
    functionName: 'getValue',
  })

  const { writeContract } = useWriteContract()

  const setValue = (newValue: bigint) => {
    writeContract({
      address: CONTRACT_ADDRESS,
      abi: contractAbi,
      functionName: 'setValue',
      args: [newValue],
    })
  }

  return { value, setValue }
}
```

---

## Troubleshooting

### Common Issues

#### Backend Issues

**Database Connection Error**
```bash
# Check MongoDB is running
docker ps | grep mongo

# Check connection string
echo $MONGODB_URI

# Test connection
go run cmd/app/main.go --test-db
```

**JWT Token Issues**
```bash
# Check JWT secret is set
echo $JWT_SECRET

# Verify token format
curl -H "Authorization: Bearer your-token" http://localhost:8080/api/v1/auth/session
```

#### Frontend Issues

**Web3 Connection Issues**
```typescript
// Check WalletConnect project ID
console.log(process.env.NEXT_PUBLIC_WALLETCONNECT_PROJECT_ID)

// Check network configuration
import { useAccount } from 'wagmi'
const { chain } = useAccount()
console.log('Current chain:', chain)
```

**API Connection Issues**
```typescript
// Check API URL
console.log(process.env.NEXT_PUBLIC_API_URL)

// Test API connection
fetch(`${process.env.NEXT_PUBLIC_API_URL}/health`)
  .then(res => res.json())
  .then(console.log)
```

#### Smart Contract Issues

**Compilation Errors**
```bash
# Check Solidity version
forge --version

# Clean and rebuild
forge clean
forge build
```

**Deployment Issues**
```bash
# Check RPC connection
cast client --rpc-url $ETH_RPC_URL

# Check account balance
cast balance $YOUR_ADDRESS --rpc-url $ETH_RPC_URL
```

### Debug Mode

Enable debug logging:

```bash
# Backend
export GIN_MODE=debug
export LOG_LEVEL=debug

# Frontend
export NODE_ENV=development
export NEXT_PUBLIC_DEBUG=true
```

---

## Best Practices

### Security

1. **Environment Variables**: Never commit secrets to git
2. **Input Validation**: Validate all user inputs
3. **Rate Limiting**: Implement appropriate rate limits
4. **HTTPS**: Always use HTTPS in production
5. **CORS**: Configure CORS properly for your domain

### Performance

1. **Database Indexing**: Add indexes for frequently queried fields
2. **Caching**: Implement Redis caching for frequently accessed data
3. **CDN**: Use CDN for static assets
4. **Bundle Optimization**: Optimize frontend bundle size

### Development

1. **Testing**: Write tests for all new features
2. **Documentation**: Document all API endpoints and components
3. **Code Review**: Use pull requests for code review
4. **Linting**: Use consistent code formatting

### Deployment

1. **Health Checks**: Implement health check endpoints
2. **Monitoring**: Set up application monitoring
3. **Logging**: Implement structured logging
4. **Backup**: Regular database backups

This guide provides a comprehensive foundation for using and extending the Web3 Boilerplate project. For specific questions or advanced use cases, refer to the individual component documentation or create an issue in the project repository.
