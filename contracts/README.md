# Payverge Smart Contracts

**Comprehensive payment processing platform with referral system and profit distribution built on Ethereum.**

Payverge is a decentralized payment platform that enables businesses to:
- Process payments with split billing capabilities
- Implement referral programs with tiered rewards
- Automatically distribute profits to stakeholders
- Manage business registrations with configurable fees

## Architecture

The Payverge ecosystem consists of three main contracts:

- **PayvergePayments**: Core payment processing with split billing and business registration
- **PayvergeReferrals**: Claim-based referral system with Basic (10%/10%) and Premium (15%/15%) tiers
- **PayvergeProfitSplit**: Automated profit distribution to up to 50 beneficiaries

## Documentation

- [Referral System Documentation](./docs/REFERRAL_SYSTEM.md)
- [Profit Split System Documentation](./docs/PROFIT_SPLIT_SYSTEM.md)
- [Foundry Book](https://book.getfoundry.sh/)

## Usage

### Build

```shell
$ forge build
```

### Test

```shell
$ forge test
```

### Format

```shell
$ forge fmt
```

### Gas Snapshots

```shell
$ forge snapshot
```

### Anvil

```shell
$ anvil
```

### Deploy

Deploy the entire Payverge ecosystem (all three contracts):

```shell
# Set environment variables
export PRIVATE_KEY=0x...
export USDC_TOKEN_ADDRESS=0x...
export PLATFORM_FEE_RECIPIENT=0x...
export PLATFORM_FEE_BPS=200
export BILL_CREATOR_ADDRESS=0x...
export REGISTRATION_FEE=0

# Deploy all contracts
$ forge script script/DeployPayvergePayments.s.sol:DeployPayvergeEcosystem --rpc-url <your_rpc_url> --broadcast --verify
```

### Upgrade Contracts

Upgrade all contracts:
```shell
# Set additional environment variables
export PAYVERGE_CONTRACT_ADDRESS=0x...
export PAYVERGE_REFERRALS_ADDRESS=0x...
export PAYVERGE_PROFIT_SPLIT_ADDRESS=0x...
export UPGRADE_TYPE=3  # 0=Payments, 1=Referrals, 2=ProfitSplit, 3=All

$ forge script script/UpgradePayvergeContracts.s.sol --rpc-url <your_rpc_url> --broadcast
```

Upgrade individual contract:
```shell
export PROXY_ADDRESS=0x...
export CONTRACT_TYPE=payments  # payments, referrals, or profit_split

$ forge script script/UpgradeIndividualContract.s.sol --rpc-url <your_rpc_url> --broadcast
```

### Cast

```shell
$ cast <subcommand>
```

### Help

```shell
$ forge --help
$ anvil --help
$ cast --help
```

## Features

### PayvergePayments
- **Split Billing**: Create bills with multiple participants and custom amount assignments
- **Business Registration**: Configurable registration fees with timelock protection
- **Payment Processing**: Unified payment system with USDC support
- **Rate Limiting**: Spam protection and nonce-based replay protection
- **Role-Based Access**: Admin, upgrader, and bill manager roles

### PayvergeReferrals
- **Tiered System**: Basic (10%/10%) and Premium (15%/15%) referrer tiers
- **Claim-Based Commissions**: Referrers manually claim earned rewards
- **Code Management**: Unique 6-12 character alphanumeric referral codes
- **Registration Fees**: $10 Basic, $25 Premium referrer registration
- **Comprehensive Tracking**: Full audit trail of referrals and commissions

### PayvergeProfitSplit
- **Flexible Distribution**: Support for up to 50 beneficiaries
- **Percentage-Based**: Custom profit sharing percentages (basis points)
- **Automatic Payments**: Direct transfers to beneficiary wallets
- **Role Management**: Admin, distributor, and upgrader roles
- **Emergency Controls**: Pause functionality and emergency withdrawals

## Testing

Run all tests:
```shell
$ forge test
```

Run specific test suite:
```shell
$ forge test --match-contract PayvergePayments
$ forge test --match-contract PayvergeReferrals
$ forge test --match-contract PayvergeProfitSplit
```

Run with gas reporting:
```shell
$ forge test --gas-report
```

Generate coverage report:
```shell
$ forge coverage
```

## Security

- **Upgradeable Contracts**: UUPS proxy pattern for all contracts
- **Access Control**: OpenZeppelin role-based permissions
- **Reentrancy Protection**: NonReentrant modifiers on critical functions
- **Pause Functionality**: Emergency pause capabilities
- **Comprehensive Testing**: 140+ tests with 100% coverage

## Contract Addresses

After deployment, update your environment with the contract addresses:

```shell
# Main contracts (use these for frontend integration)
export PAYVERGE_CONTRACT_ADDRESS=0x...      # PayvergePayments proxy
export PAYVERGE_REFERRALS_ADDRESS=0x...     # PayvergeReferrals proxy
export PAYVERGE_PROFIT_SPLIT_ADDRESS=0x...  # PayvergeProfitSplit proxy
export USDC_CONTRACT_ADDRESS=0x...          # USDC token contract
```

## License

MIT
