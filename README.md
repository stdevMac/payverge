# One-Click USDC Invoice Generator

A comprehensive web application for creating professional crypto invoices with automatic USDC payments, built with smart contracts, Golang backend, and Next.js frontend.

## 🚀 Features

- **Smart Contract Payments**: Automatic 1% fee split with 99% going directly to creators
- **Professional Invoices**: Clean, branded invoice creation and sharing
- **Email Notifications**: Automated invoice delivery and payment confirmations via Postmark
- **QR Code Support**: Mobile-friendly payment links and QR codes
- **Real-time Updates**: WebSocket integration for instant payment notifications
- **Multi-chain Support**: Ethereum mainnet with Polygon/Base expansion planned
- **Wallet Integration**: RainbowKit + Wagmi for seamless Web3 connectivity

## 🏗️ Architecture

### Smart Contract (`/contracts`)
- **InvoiceGenerator.sol**: Core contract handling invoice creation and payments
- **1% Platform Fee**: Automatic fee deduction with direct creator payouts
- **Event Emission**: Real-time blockchain events for payment tracking

### Backend (`/backend`)
- **Golang + Gin**: High-performance API server
- **PostgreSQL**: Invoice and payment data storage
- **Event Listener**: Blockchain event monitoring via go-ethereum
- **Email Service**: Postmark integration for notifications
- **QR/Link Generation**: Payment link and QR code services

### Frontend (`/frontend`)
- **Next.js 14**: Modern React framework with App Router
- **RainbowKit + Wagmi**: Web3 wallet connection and contract interaction
- **Tailwind CSS**: Responsive, modern UI design
- **Real-time Updates**: WebSocket connection for live payment status

## 🛠️ Setup Instructions

### Prerequisites
- Node.js 18+
- Go 1.21+
- PostgreSQL 14+
- Ethereum wallet with testnet/mainnet access

### 1. Environment Configuration
```bash
cp .env.example .env
# Edit .env with your configuration
```

### 2. Database Setup
```bash
# Create PostgreSQL database
createdb invoice_generator

# Database will be auto-migrated on first run
```

### 3. Smart Contract Deployment
```bash
cd contracts
npm install
npx hardhat compile

# Deploy to testnet (Sepolia)
npx hardhat run scripts/deploy.js --network sepolia

# Update CONTRACT_ADDRESS in .env
```

### 4. Backend Setup
```bash
cd backend
go mod download
go run main.go
# Server runs on http://localhost:8080
```

### 5. Frontend Setup
```bash
cd frontend
npm install
npm run dev
# Frontend runs on http://localhost:3000
```

## 📝 Usage

### Creating an Invoice
1. Connect your wallet on the homepage
2. Fill out the invoice form with:
   - Title and description
   - Amount in USDC
   - Optional client email and due date
3. Submit to create invoice and get shareable payment link

### Paying an Invoice
1. Visit the payment link
2. Connect wallet and approve USDC spending
3. Enter payment amount (supports partial payments)
4. Confirm transaction - 99% goes to creator, 1% platform fee

### Managing Invoices
- View all your invoices in the dashboard
- Track payment status in real-time
- Copy payment links and download QR codes
- View transaction history on Etherscan

## 🔧 API Endpoints

### Invoices
- `POST /api/v1/invoices` - Create new invoice
- `GET /api/v1/invoices/:id` - Get invoice details
- `GET /api/v1/invoices?creator=:address` - Get invoices by creator
- `DELETE /api/v1/invoices/:id` - Cancel invoice

### Payments
- `GET /api/v1/invoices/:id/payments` - Get payment history
- `GET /api/v1/invoices/:id/metadata` - Get invoice metadata

### WebSocket
- `ws://localhost:8080/ws` - Real-time updates

## 🚀 Deployment

### Backend Deployment
```bash
# Build binary
go build -o invoice-generator main.go

# Run with production environment
./invoice-generator
```

### Frontend Deployment
```bash
# Build for production
npm run build
npm run start
```

### Smart Contract Deployment
```bash
# Deploy to mainnet
npx hardhat run scripts/deploy.js --network ethereum
npx hardhat verify --network ethereum DEPLOYED_ADDRESS
```

## 🔐 Security Features

- **Non-custodial**: Payments flow directly through smart contracts
- **ReentrancyGuard**: Protection against reentrancy attacks
- **Input Validation**: Comprehensive validation on all inputs
- **Rate Limiting**: API rate limiting for abuse prevention
- **CORS Protection**: Proper cross-origin request handling

## 📊 Monitoring

### Health Checks
- `GET /health` - Backend health status
- Database connection monitoring
- Smart contract interaction status

### Logging
- Structured logging with request tracing
- Payment event logging
- Error tracking and alerting

## 🛣️ Roadmap

### Phase 1 (MVP) ✅
- [x] Smart contract with fee handling
- [x] Invoice creation and payment flow
- [x] Email notifications
- [x] Basic dashboard

### Phase 2
- [ ] Multi-token support (DAI, USDT)
- [ ] Recurring invoices/subscriptions
- [ ] PDF invoice generation
- [ ] Advanced analytics

### Phase 3
- [ ] Multi-chain deployment (Polygon, Base)
- [ ] Mobile app
- [ ] White-label solutions
- [ ] API for third-party integrations

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## 📄 License

MIT License - see LICENSE file for details

## 🆘 Support

- Documentation: [Link to docs]
- Discord: [Community link]
- Email: support@invoicegen.com

---

**Built with ❤️ for the Web3 community**
