#!/bin/bash

# One-Click USDC Invoice Generator Startup Script
echo "🚀 Starting Invoice Generator Application..."

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Check if .env file exists
if [ ! -f .env ]; then
    echo -e "${YELLOW}⚠️  .env file not found. Creating from template...${NC}"
    cp .env.example .env
    echo -e "${YELLOW}📝 Please edit .env file with your configuration before continuing.${NC}"
    exit 1
fi

# Function to check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Check prerequisites
echo -e "${BLUE}🔍 Checking prerequisites...${NC}"

if ! command_exists node; then
    echo -e "${RED}❌ Node.js not found. Please install Node.js 18+${NC}"
    exit 1
fi

if ! command_exists go; then
    echo -e "${RED}❌ Go not found. Please install Go 1.21+${NC}"
    exit 1
fi

if ! command_exists psql; then
    echo -e "${YELLOW}⚠️  PostgreSQL client not found. Make sure PostgreSQL is installed.${NC}"
fi

echo -e "${GREEN}✅ Prerequisites check completed${NC}"

# Start PostgreSQL (if using Docker)
if command_exists docker && command_exists docker-compose; then
    echo -e "${BLUE}🐳 Starting PostgreSQL with Docker...${NC}"
    docker-compose up -d postgres
    sleep 5
fi

# Install and setup contracts
echo -e "${BLUE}📄 Setting up smart contracts...${NC}"
cd contracts
if [ ! -d node_modules ]; then
    npm install
fi
npm run compile
cd ..

# Setup backend
echo -e "${BLUE}🔧 Setting up backend...${NC}"
cd backend
if [ ! -f go.sum ]; then
    go mod download
fi

# Create static directory for QR codes
mkdir -p static

# Start backend in background
echo -e "${BLUE}🚀 Starting backend server...${NC}"
go run main.go &
BACKEND_PID=$!
cd ..

# Wait for backend to start
sleep 5

# Setup frontend
echo -e "${BLUE}🎨 Setting up frontend...${NC}"
cd frontend
if [ ! -d node_modules ]; then
    npm install
fi

# Start frontend
echo -e "${BLUE}🚀 Starting frontend...${NC}"
npm run dev &
FRONTEND_PID=$!
cd ..

# Wait a moment for services to start
sleep 3

echo -e "${GREEN}✅ Application started successfully!${NC}"
echo ""
echo -e "${BLUE}📱 Frontend:${NC} http://localhost:3000"
echo -e "${BLUE}🔧 Backend API:${NC} http://localhost:8080"
echo -e "${BLUE}💾 Health Check:${NC} http://localhost:8080/health"
echo ""
echo -e "${YELLOW}📝 Next Steps:${NC}"
echo "1. Deploy smart contract: cd contracts && npx hardhat run scripts/deploy.js --network sepolia"
echo "2. Update CONTRACT_ADDRESS in .env file"
echo "3. Configure Postmark API key for emails"
echo "4. Set up Alchemy/Infura RPC URLs"
echo ""
echo -e "${GREEN}🎉 Ready to create invoices and get paid in USDC!${NC}"
echo ""
echo "Press Ctrl+C to stop all services"

# Function to cleanup on exit
cleanup() {
    echo -e "\n${YELLOW}🛑 Stopping services...${NC}"
    kill $BACKEND_PID 2>/dev/null
    kill $FRONTEND_PID 2>/dev/null
    if command_exists docker-compose; then
        docker-compose down
    fi
    echo -e "${GREEN}✅ Services stopped${NC}"
    exit 0
}

# Trap Ctrl+C
trap cleanup INT

# Wait for user to stop
wait
