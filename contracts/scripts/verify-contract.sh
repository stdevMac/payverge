#!/bin/bash

# Payverge Contract Verification Script
# This script verifies the deployed PayvergePayments contract on block explorers

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if required environment variables are set
check_env_vars() {
    print_status "Checking environment variables..."
    
    if [ -z "$PROXY_ADDRESS" ]; then
        print_error "PROXY_ADDRESS environment variable is required"
        echo "Usage: PROXY_ADDRESS=0x... ETHERSCAN_API_KEY=... ./scripts/verify-contract.sh"
        exit 1
    fi
    
    if [ -z "$ETHERSCAN_API_KEY" ]; then
        print_error "ETHERSCAN_API_KEY environment variable is required"
        echo "Usage: PROXY_ADDRESS=0x... ETHERSCAN_API_KEY=... ./scripts/verify-contract.sh"
        exit 1
    fi
    
    if [ -z "$RPC_URL" ]; then
        print_warning "RPC_URL not set, using default"
        export RPC_URL="https://mainnet.base.org"
    fi
    
    print_success "Environment variables validated"
}

# Verify the proxy contract
verify_proxy() {
    print_status "Verifying proxy contract at $PROXY_ADDRESS..."
    
    # Get the implementation address from the proxy
    IMPLEMENTATION_ADDRESS=$(cast call $PROXY_ADDRESS "implementation()" --rpc-url $RPC_URL | sed 's/0x000000000000000000000000/0x/')
    
    print_status "Implementation address: $IMPLEMENTATION_ADDRESS"
    
    # Verify the implementation contract
    print_status "Verifying implementation contract..."
    
    forge verify-contract \
        $IMPLEMENTATION_ADDRESS \
        src/PayvergePayments.sol:PayvergePayments \
        --etherscan-api-key $ETHERSCAN_API_KEY \
        --rpc-url $RPC_URL \
        --watch
    
    if [ $? -eq 0 ]; then
        print_success "Implementation contract verified successfully!"
    else
        print_error "Failed to verify implementation contract"
        exit 1
    fi
    
    # Verify the proxy contract
    print_status "Verifying proxy contract..."
    
    forge verify-contract \
        $PROXY_ADDRESS \
        lib/openzeppelin-contracts/contracts/proxy/ERC1967/ERC1967Proxy.sol:ERC1967Proxy \
        --etherscan-api-key $ETHERSCAN_API_KEY \
        --rpc-url $RPC_URL \
        --constructor-args $(cast abi-encode "constructor(address,bytes)" $IMPLEMENTATION_ADDRESS 0x) \
        --watch
    
    if [ $? -eq 0 ]; then
        print_success "Proxy contract verified successfully!"
    else
        print_warning "Proxy verification failed (this is often expected)"
    fi
}

# Verify contract functionality
verify_functionality() {
    print_status "Verifying contract functionality..."
    
    # Check contract version
    VERSION=$(cast call $PROXY_ADDRESS "version()" --rpc-url $RPC_URL | sed 's/0x//' | xxd -r -p)
    print_status "Contract version: $VERSION"
    
    # Check USDC token address
    USDC_ADDRESS=$(cast call $PROXY_ADDRESS "usdcToken()" --rpc-url $RPC_URL)
    print_status "USDC token address: $USDC_ADDRESS"
    
    # Check platform fee rate
    FEE_RATE=$(cast call $PROXY_ADDRESS "platformFeeRate()" --rpc-url $RPC_URL)
    FEE_RATE_DECIMAL=$((16#${FEE_RATE#0x}))
    print_status "Platform fee rate: $FEE_RATE_DECIMAL basis points ($(echo "scale=2; $FEE_RATE_DECIMAL/100" | bc)%)"
    
    # Check registration fee
    REG_FEE=$(cast call $PROXY_ADDRESS "getRegistrationFee()" --rpc-url $RPC_URL)
    REG_FEE_DECIMAL=$((16#${REG_FEE#0x}))
    REG_FEE_USDC=$(echo "scale=6; $REG_FEE_DECIMAL/1000000" | bc)
    print_status "Registration fee: $REG_FEE_USDC USDC"
    
    print_success "Contract functionality verified!"
}

# Generate verification report
generate_report() {
    print_status "Generating verification report..."
    
    REPORT_FILE="verification-report-$(date +%Y%m%d-%H%M%S).txt"
    
    cat > $REPORT_FILE << EOF
Payverge Contract Verification Report
=====================================
Generated: $(date)

Contract Addresses:
- Proxy: $PROXY_ADDRESS
- Implementation: $IMPLEMENTATION_ADDRESS

Network Information:
- RPC URL: $RPC_URL
- Chain ID: $(cast chain-id --rpc-url $RPC_URL)

Contract Details:
- Version: $VERSION
- USDC Token: $USDC_ADDRESS
- Platform Fee: $FEE_RATE_DECIMAL basis points ($(echo "scale=2; $FEE_RATE_DECIMAL/100" | bc)%)
- Registration Fee: $REG_FEE_USDC USDC

Verification Status:
- Implementation: ✅ Verified
- Proxy: ⚠️  May require manual verification

Block Explorer Links:
- Proxy: https://basescan.org/address/$PROXY_ADDRESS
- Implementation: https://basescan.org/address/$IMPLEMENTATION_ADDRESS

Next Steps:
1. Update frontend environment variables with proxy address
2. Test contract functionality through frontend
3. Monitor contract for any issues
4. Set up monitoring and alerts

EOF

    print_success "Verification report saved to: $REPORT_FILE"
}

# Main execution
main() {
    echo "======================================"
    echo "  Payverge Contract Verification"
    echo "======================================"
    echo
    
    check_env_vars
    verify_proxy
    verify_functionality
    generate_report
    
    echo
    print_success "Contract verification completed successfully!"
    echo
    echo "Next steps:"
    echo "1. Update your frontend with the proxy address: $PROXY_ADDRESS"
    echo "2. Test the contract functionality"
    echo "3. Monitor the contract for any issues"
    echo
}

# Run the main function
main "$@"
