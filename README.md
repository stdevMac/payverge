# Web3 Boilerplate

A comprehensive, production-ready boilerplate for building Web3 applications with modern authentication, blockchain integration, and scalable architecture.

## 🚀 Features

- **SIWE Authentication**: Sign-In with Ethereum using Reown AppKit + Wagmi
- **Multi-chain Support**: Ethereum, Polygon, Base with easy chain switching
- **Modern Stack**: Next.js 14, Golang backend, MongoDB database
- **Web3 Integration**: Contract interaction, transaction handling, wallet management
- **File Storage**: AWS S3 integration with dual bucket support
- **Notifications**: Email (Postmark) and Telegram bot integration
- **Real-time Updates**: WebSocket support for live data
- **Monitoring**: Prometheus metrics and PostHog analytics
- **Containerized**: Docker Compose for easy development and deployment

## 🏗️ Architecture

### Smart Contracts (`/contracts`)
- **Foundry Setup**: Modern Solidity development environment
- **Contract Templates**: ERC20, ERC721, and governance contract examples
- **Deployment Scripts**: Automated deployment to multiple networks
- **Testing Suite**: Comprehensive contract testing with Foundry

### Backend (`/backend`)
- **Golang + Gin**: High-performance REST API server
- **MongoDB**: Document database with connection pooling
- **SIWE Authentication**: Ethereum-based user authentication
- **Web3 Integration**: Ethereum client for blockchain interactions
- **File Upload**: S3 integration for secure file storage
- **Notifications**: Multi-channel notification system

### Frontend (`/frontend`)
- **Next.js 14**: Modern React framework with App Router
- **Reown AppKit**: Advanced wallet connection and management
- **Wagmi + Viem**: Type-safe Ethereum interactions
- **NextUI + Tailwind**: Beautiful, responsive UI components
- **Internationalization**: Multi-language support with next-intl
- **State Management**: Zustand + React Query for optimal performance

## 🛠️ Quick Start

### Prerequisites
- Node.js 18+
- Go 1.21+
- MongoDB 7.0+
- Docker & Docker Compose

### 1. Environment Setup
```bash
cp .env.sample .env
# Edit .env with your configuration
```

### 2. Start with Docker (Recommended)
```bash
npm run docker:up
```

### 3. Manual Setup

#### Database
```bash
# Start MongoDB
docker run -d -p 27017:27017 --name mongodb mongo:7.0
```

#### Smart Contracts
```bash
cd contracts
forge install
forge build
forge test
```

#### Backend
```bash
cd backend
go mod download
go run cmd/app/main.go
# Server runs on http://localhost:8080
```

#### Frontend
```bash
cd frontend
npm install
npm run dev
# Frontend runs on http://localhost:3000
```

## 📝 Configuration

### Environment Variables

#### Required
```env
# Database
MONGO_ROOT_USERNAME=admin
MONGO_ROOT_PASSWORD=your_password
MONGO_DATABASE=your_app_name

# Web3
NEXT_PUBLIC_PROJECT_ID=your_wallet_connect_project_id
NEXT_PUBLIC_ALCHEMY_ID=your_alchemy_api_key
RPC_URL=https://eth-mainnet.g.alchemy.com/v2/your-key
CHAIN_ID=1

# Authentication
FAUCET_PRIVATE_KEY=your_private_key_for_testnet_faucet
```

#### Optional
```env
# AWS S3
AWS_ACCESS_KEY=your_aws_access_key
AWS_SECRET_KEY=your_aws_secret_key
S3_BUCKET=your_bucket_name

# Notifications
POSTMARK_API_KEY=your_postmark_key
TELEGRAM_TOKEN=your_telegram_bot_token

# Analytics
POSTHOG_API_KEY=your_posthog_key
```

## 🔧 API Endpoints

### Authentication
- `POST /api/v1/auth/challenge` - Generate SIWE challenge
- `POST /api/v1/auth/signin` - Sign in with Ethereum
- `POST /api/v1/auth/signout` - Sign out user
- `GET /api/v1/auth/session` - Get current session

### User Management
- `GET /api/v1/inside/get_user/:address` - Get user profile
- `PUT /api/v1/inside/update_user` - Update user profile
- `PUT /api/v1/inside/settings/notifications` - Update notification preferences

### File Upload
- `POST /api/v1/inside/upload` - Upload file to S3
- `POST /api/v1/inside/upload_protected` - Upload to protected bucket

### Blockchain
- `POST /api/v1/inside/faucet` - Request testnet tokens
- `GET /api/v1/inside/faucet/check/:address` - Check faucet availability

### Admin
- `GET /api/v1/admin/get_all_users` - Get all users (admin only)
- `GET /api/v1/admin/get_subscribers` - Get email subscribers

## 🚀 Deployment

### Production Build
```bash
# Build all components
npm run build

# Or build individually
npm run build:contracts
npm run build:backend
npm run build:frontend
```

### Docker Deployment
```bash
# Production deployment
docker-compose -f docker-compose.prod.yml up -d
```

### Contract Deployment
```bash
cd contracts
# Deploy to mainnet
forge script script/Deploy.s.sol --rpc-url $ETHEREUM_RPC_URL --private-key $PRIVATE_KEY --broadcast --verify
```

## 🔐 Security Features

- **Non-custodial Authentication**: SIWE-based login without storing private keys
- **Input Validation**: Comprehensive validation on all API endpoints
- **Rate Limiting**: Built-in protection against abuse
- **CORS Protection**: Proper cross-origin request handling
- **Environment Isolation**: Separate configurations for development/production

## 🧪 Testing

```bash
# Run all tests
npm run test

# Backend tests
cd backend && go test ./...

# Frontend tests
cd frontend && npm run test

# Contract tests
cd contracts && forge test
```

## 📊 Monitoring & Analytics

- **Health Checks**: `/health` endpoint for service monitoring
- **Metrics**: Prometheus metrics at `/metrics`
- **Analytics**: PostHog integration for user analytics
- **Logging**: Structured logging with request tracing

## 🛣️ Roadmap

### ✅ Completed
- [x] SIWE authentication system
- [x] Multi-chain wallet integration
- [x] File upload and storage
- [x] Notification system
- [x] Docker containerization

### 🔄 In Progress
- [ ] Smart contract templates
- [ ] Comprehensive testing suite
- [ ] API documentation
- [ ] Performance optimizations

### 📋 Planned
- [ ] Multi-chain deployment scripts
- [ ] DeFi integration templates
- [ ] Mobile app support
- [ ] Advanced analytics dashboard

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/amazing-feature`
3. Make your changes and add tests
4. Commit your changes: `git commit -m 'Add amazing feature'`
5. Push to the branch: `git push origin feature/amazing-feature`
6. Open a Pull Request

## 📄 License

MIT License - see [LICENSE](LICENSE) file for details.

## 🆘 Support

- **Documentation**: Check the `/docs` folder for detailed guides
- **Issues**: Report bugs and request features via GitHub Issues
- **Discussions**: Join community discussions in GitHub Discussions

---

**Built for the Web3 community** 🌐⚡
