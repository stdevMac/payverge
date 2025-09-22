# PayvergePayments Contract Upgrade Procedures

## 🔄 Overview

The PayvergePayments contract uses OpenZeppelin's UUPS (Universal Upgradeable Proxy Standard) pattern, allowing for secure upgrades while preserving state and user funds.

## 🔐 Security Model

### Access Control
- Only addresses with `UPGRADER_ROLE` can perform upgrades
- By default, the deployer receives all admin roles including `UPGRADER_ROLE`
- Multi-signature wallet recommended for production upgrades

### Upgrade Safety
- All upgrades must be compatible with existing storage layout
- State variables cannot be removed or reordered
- New state variables must be added at the end

## 📋 Pre-Upgrade Checklist

### 1. Code Review & Testing
- [ ] New implementation thoroughly tested
- [ ] Storage layout compatibility verified
- [ ] All existing tests pass with new implementation
- [ ] New functionality tested comprehensively
- [ ] Security audit completed (for major changes)

### 2. Deployment Preparation
- [ ] New implementation contract compiled
- [ ] Gas costs estimated
- [ ] Upgrade transaction prepared
- [ ] Rollback plan documented
- [ ] Team notified of upgrade schedule

### 3. Environment Setup
- [ ] Upgrader private key secured
- [ ] RPC endpoints configured
- [ ] Block explorer API keys ready
- [ ] Monitoring systems prepared

## 🚀 Upgrade Process

### Step 1: Deploy New Implementation

```bash
# Set environment variables
export PRIVATE_KEY=0x...
export RPC_URL=https://mainnet.base.org
export ETHERSCAN_API_KEY=...

# Deploy new implementation
forge create src/PayvergePayments.sol:PayvergePayments \
    --rpc-url $RPC_URL \
    --private-key $PRIVATE_KEY \
    --verify \
    --etherscan-api-key $ETHERSCAN_API_KEY

# Save the deployed address
export NEW_IMPLEMENTATION=0x...
```

### Step 2: Verify New Implementation

```bash
# Verify the implementation contract
forge verify-contract $NEW_IMPLEMENTATION \
    src/PayvergePayments.sol:PayvergePayments \
    --etherscan-api-key $ETHERSCAN_API_KEY \
    --rpc-url $RPC_URL

# Test the implementation (without proxy)
cast call $NEW_IMPLEMENTATION "version()" --rpc-url $RPC_URL
```

### Step 3: Prepare Upgrade Transaction

```bash
# Get current proxy address
export PROXY_ADDRESS=0x...

# Verify current implementation
CURRENT_IMPL=$(cast call $PROXY_ADDRESS "implementation()" --rpc-url $RPC_URL)
echo "Current implementation: $CURRENT_IMPL"
echo "New implementation: $NEW_IMPLEMENTATION"

# Verify upgrader has permission
cast call $PROXY_ADDRESS "hasRole(bytes32,address)" \
    $(cast keccak "UPGRADER_ROLE") \
    $UPGRADER_ADDRESS \
    --rpc-url $RPC_URL
```

### Step 4: Execute Upgrade

```bash
# Execute the upgrade
cast send $PROXY_ADDRESS \
    "upgradeTo(address)" \
    $NEW_IMPLEMENTATION \
    --rpc-url $RPC_URL \
    --private-key $UPGRADER_PRIVATE_KEY \
    --gas-limit 200000

# Verify upgrade was successful
NEW_IMPL=$(cast call $PROXY_ADDRESS "implementation()" --rpc-url $RPC_URL)
echo "Updated implementation: $NEW_IMPL"

# Verify new version
cast call $PROXY_ADDRESS "version()" --rpc-url $RPC_URL
```

### Step 5: Post-Upgrade Verification

```bash
# Test critical functions
cast call $PROXY_ADDRESS "usdcToken()" --rpc-url $RPC_URL
cast call $PROXY_ADDRESS "platformFeeRate()" --rpc-url $RPC_URL
cast call $PROXY_ADDRESS "getRegistrationFee()" --rpc-url $RPC_URL

# Test business registration (if applicable)
# Test bill creation (if applicable)
# Test payment processing (if applicable)
```

## 🔧 Upgrade Script Template

Create `script/UpgradePayvergePayments.s.sol`:

```solidity
// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

import "forge-std/Script.sol";
import "../src/PayvergePayments.sol";

contract UpgradePayvergePayments is Script {
    function run() external {
        uint256 upgraderPrivateKey = vm.envUint("UPGRADER_PRIVATE_KEY");
        address proxyAddress = vm.envAddress("PROXY_ADDRESS");
        
        vm.startBroadcast(upgraderPrivateKey);
        
        // Deploy new implementation
        PayvergePayments newImplementation = new PayvergePayments();
        console.log("New implementation deployed at:", address(newImplementation));
        
        // Upgrade proxy to new implementation
        PayvergePayments proxy = PayvergePayments(proxyAddress);
        proxy.upgradeTo(address(newImplementation));
        
        console.log("Upgrade completed successfully");
        console.log("Proxy address:", proxyAddress);
        console.log("New implementation:", address(newImplementation));
        console.log("New version:", proxy.version());
        
        vm.stopBroadcast();
    }
}
```

## ⚠️ Emergency Procedures

### Emergency Pause
If critical issues are discovered post-upgrade:

```bash
# Pause the contract immediately
cast send $PROXY_ADDRESS \
    "pause()" \
    --rpc-url $RPC_URL \
    --private-key $ADMIN_PRIVATE_KEY
```

### Emergency Rollback
If the upgrade needs to be reverted:

```bash
# Deploy previous implementation version
forge create src/PayvergePaymentsV4.sol:PayvergePayments \
    --rpc-url $RPC_URL \
    --private-key $PRIVATE_KEY

# Upgrade back to previous implementation
cast send $PROXY_ADDRESS \
    "upgradeTo(address)" \
    $PREVIOUS_IMPLEMENTATION \
    --rpc-url $RPC_URL \
    --private-key $UPGRADER_PRIVATE_KEY
```

## 📊 Storage Layout Compatibility

### Current Storage Layout (v5.0.0)
```solidity
// Slot 0-50: OpenZeppelin upgradeable contracts
// Slot 51: usdcToken
// Slot 52: platformTreasury  
// Slot 53: platformFeeRate
// Slot 54: billCreatorAddress
// Slot 55: businessRegistrationFee
// Slot 56: feeUpdateDelay
// Slot 57: pendingFeeRate
// Slot 58: feeUpdateTimestamp
// Slot 59: pendingRegistrationFee
// Slot 60: registrationFeeUpdateTimestamp
// Slot 61+: mappings and arrays
```

### Adding New Storage Variables
Always add new variables at the end:

```solidity
contract PayvergePaymentsV6 is PayvergePaymentsV5 {
    // ✅ SAFE: Adding new variables at the end
    uint256 public newFeature;
    mapping(address => bool) public newMapping;
    
    // ❌ UNSAFE: Never modify existing variables
    // uint256 public platformFeeRate; // Don't change existing variables
}
```

## 🧪 Testing Upgrades

### Local Testing
```bash
# Deploy proxy with current implementation
forge script script/DeployPayvergePayments.s.sol --fork-url $RPC_URL

# Deploy new implementation
forge create src/PayvergePaymentsV6.sol:PayvergePayments --fork-url $RPC_URL

# Test upgrade
cast send $PROXY_ADDRESS "upgradeTo(address)" $NEW_IMPL --fork-url $RPC_URL

# Verify functionality
cast call $PROXY_ADDRESS "version()" --fork-url $RPC_URL
```

### Testnet Testing
1. Deploy to Base Sepolia first
2. Test all functionality thoroughly
3. Verify upgrade process works correctly
4. Only then proceed to mainnet

## 📋 Upgrade Checklist Template

```markdown
## Upgrade Checklist - Version X.X.X

### Pre-Upgrade
- [ ] Code review completed
- [ ] Tests passing (X/X)
- [ ] Security audit completed
- [ ] Storage layout verified
- [ ] Gas costs estimated
- [ ] Testnet deployment successful
- [ ] Team notified

### Deployment
- [ ] New implementation deployed
- [ ] Implementation verified on block explorer
- [ ] Upgrade transaction prepared
- [ ] Upgrade executed successfully
- [ ] New version verified

### Post-Upgrade
- [ ] All functions working correctly
- [ ] Frontend integration tested
- [ ] Monitoring systems updated
- [ ] Documentation updated
- [ ] Team notified of completion

### Rollback Plan (if needed)
- [ ] Previous implementation address: 0x...
- [ ] Rollback transaction prepared
- [ ] Emergency contacts notified
```

## 🔍 Monitoring Post-Upgrade

### Key Metrics to Monitor
- Transaction success rates
- Gas usage patterns
- Error rates and types
- Platform fee collection
- Business registration rates
- Payment processing volume

### Alerts to Set Up
- Failed transactions above threshold
- Unusual gas consumption
- Contract paused events
- Large fee changes
- Admin role changes

## 📞 Emergency Contacts

Maintain a list of key personnel for upgrade emergencies:
- Lead Developer: [contact info]
- DevOps Engineer: [contact info]
- Security Auditor: [contact info]
- Business Stakeholder: [contact info]

## 📚 Additional Resources

- [OpenZeppelin Upgrades Documentation](https://docs.openzeppelin.com/upgrades-plugins/1.x/)
- [UUPS Pattern Explanation](https://eips.ethereum.org/EIPS/eip-1822)
- [Storage Layout Best Practices](https://docs.openzeppelin.com/upgrades-plugins/1.x/writing-upgradeable)

---

**Remember**: Upgrades are irreversible operations that affect user funds. Always test thoroughly and have a rollback plan ready.
