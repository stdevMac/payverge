# Project Folder Structure

## Overview

This document provides a comprehensive overview of the Web3 Boilerplate project structure, explaining the purpose and contents of each directory and key files.

## Root Directory

```
blockvantage/base/
├── backend/                 # Go backend API server
├── contracts/              # Smart contracts (Foundry)
├── frontend/               # Next.js frontend application
├── docs/                   # Project documentation
├── .env.example            # Environment variables template
├── .env.sample             # Sample environment configuration
├── .gitignore              # Git ignore patterns
├── .gitmodules             # Git submodules configuration
├── CLEANUP_STEPS.md        # Development phases and cleanup tasks
├── README.md               # Project overview and setup
├── SECURITY.md             # Security guidelines and policies
├── docker-compose.yml      # Docker services configuration
├── package.json            # Root package configuration
└── start.sh                # Project startup script
```

---

## Backend Structure

```
backend/
├── cmd/
│   └── app/
│       └── main.go         # Application entry point and server setup
├── internal/
│   ├── config/
│   │   └── config.go       # Configuration management
│   ├── database/
│   │   ├── database.go     # Database connection and operations
│   │   ├── models.go       # Data models and schemas
│   │   └── migrations.go   # Database migrations
│   ├── emails/
│   │   ├── emails.go       # Email service implementation
│   │   └── templates/      # Email templates
│   ├── faucet/
│   │   └── faucet.go       # Ethereum faucet functionality
│   ├── middleware/
│   │   ├── auth.go         # Authentication middleware
│   │   ├── cors.go         # CORS configuration
│   │   ├── logging.go      # Request logging
│   │   ├── metrics.go      # Prometheus metrics
│   │   ├── ratelimit.go    # Rate limiting
│   │   └── security.go     # Security headers and validation
│   ├── metrics/
│   │   └── metrics.go      # Application metrics collection
│   ├── multisig/
│   │   └── multisig.go     # Multisig wallet operations
│   ├── notifications/
│   │   ├── notifications.go # Notification dispatcher
│   │   ├── email.go        # Email notifications
│   │   └── telegram.go     # Telegram notifications
│   ├── server/
│   │   ├── auth_handlers.go    # Authentication endpoints
│   │   ├── user_handlers.go    # User management endpoints
│   │   ├── faucet_handlers.go  # Faucet endpoints
│   │   ├── upload_handlers.go  # File upload endpoints
│   │   ├── admin_handlers.go   # Admin endpoints
│   │   ├── telegram_handlers.go # Telegram webhook handlers
│   │   └── websocket_handlers.go # WebSocket connections
│   ├── storage/
│   │   └── s3.go           # S3 storage operations
│   ├── telegram/
│   │   └── bot.go          # Telegram bot implementation
│   ├── utils/
│   │   ├── utils.go        # Utility functions
│   │   ├── crypto.go       # Cryptographic operations
│   │   └── validation.go   # Input validation
│   └── websocket/
│       └── hub.go          # WebSocket connection hub
├── scripts/
│   └── test-coverage.sh    # Test coverage script
├── Dockerfile              # Docker container configuration
├── go.mod                  # Go module dependencies
├── go.sum                  # Go module checksums
├── Makefile                # Build and test commands
├── README.md               # Backend documentation
└── docker-compose.dev.yml  # Development Docker setup
```

### Backend Key Files

- **`cmd/app/main.go`**: Server initialization, middleware setup, route registration
- **`internal/server/*_handlers.go`**: HTTP request handlers for different feature areas
- **`internal/middleware/`**: HTTP middleware for security, auth, logging, metrics
- **`internal/database/`**: MongoDB operations and data models
- **`internal/utils/utils.go`**: SIWE signature verification and utilities

---

## Frontend Structure

```
frontend/
├── .github/
│   └── workflows/
│       ├── deploy.yml      # Deployment workflow
│       └── test.yml        # Testing workflow
├── scripts/
│   ├── batchUploadToS3.js  # S3 batch upload utility
│   └── download-fonts.sh   # Font download script
├── src/
│   ├── api/
│   │   ├── auth.ts         # Authentication API calls
│   │   ├── user.ts         # User API calls
│   │   ├── faucet.ts       # Faucet API calls
│   │   └── admin.ts        # Admin API calls
│   ├── app/
│   │   ├── admin/
│   │   │   ├── layout.tsx  # Admin layout wrapper
│   │   │   ├── page.tsx    # Admin dashboard
│   │   │   ├── users/      # User management pages
│   │   │   ├── subscribers/ # Subscriber management
│   │   │   ├── multisig/   # Multisig management
│   │   │   └── codes/      # Code management
│   │   ├── auth/
│   │   │   └── page.tsx    # Authentication page
│   │   ├── dashboard/
│   │   │   └── page.tsx    # User dashboard
│   │   ├── profile/
│   │   │   └── page.tsx    # User profile page
│   │   ├── globals.css     # Global styles
│   │   ├── layout.tsx      # Root layout component
│   │   ├── loading.tsx     # Loading component
│   │   ├── not-found.tsx   # 404 page
│   │   └── page.tsx        # Home page
│   ├── components/
│   │   ├── auth/
│   │   │   ├── ConnectButton.tsx    # Wallet connection
│   │   │   ├── SignInButton.tsx     # SIWE authentication
│   │   │   └── AuthProvider.tsx     # Auth context provider
│   │   ├── dashboard/
│   │   │   ├── Dashboard.tsx        # Main dashboard
│   │   │   ├── Stats.tsx           # Statistics display
│   │   │   └── RecentActivity.tsx   # Activity feed
│   │   ├── admin/
│   │   │   ├── AdminPanel.tsx       # Admin control panel
│   │   │   ├── UserManagement.tsx   # User management
│   │   │   ├── SubscriberList.tsx   # Subscriber management
│   │   │   └── CodeGenerator.tsx    # Code generation
│   │   ├── ui/
│   │   │   ├── top-menu/
│   │   │   │   ├── TopMenu.tsx      # Main navigation
│   │   │   │   └── TopMenuMain.tsx  # Admin navigation
│   │   │   ├── sidebar/
│   │   │   │   └── Sidebar.tsx      # Sidebar navigation
│   │   │   ├── footer/
│   │   │   │   └── Footer.tsx       # Footer component
│   │   │   ├── banners/
│   │   │   │   ├── MaintenanceBanner.tsx
│   │   │   │   └── ComingSoonBanner.tsx
│   │   │   └── notifications/
│   │   │       └── NotificationCenter.tsx
│   │   ├── main/
│   │   │   ├── management/
│   │   │   │   └── Management.tsx   # Treasury management
│   │   │   ├── profile/
│   │   │   │   └── Profile.tsx      # User profile
│   │   │   └── settings/
│   │   │       └── Settings.tsx     # User settings
│   │   └── shared/
│   │       ├── LoadingSpinner.tsx   # Loading indicator
│   │       ├── ErrorBoundary.tsx    # Error handling
│   │       └── Modal.tsx           # Modal component
│   ├── context/
│   │   ├── UserContext.tsx         # User state management
│   │   ├── Web3Context.tsx         # Web3 provider context
│   │   └── NotificationContext.tsx  # Notification system
│   ├── hooks/
│   │   ├── useAuth.ts              # Authentication hook
│   │   ├── useUser.ts              # User data hook
│   │   ├── useWeb3.ts              # Web3 integration hook
│   │   └── useNotifications.ts     # Notifications hook
│   ├── lib/
│   │   ├── auth.ts                 # Auth utilities
│   │   ├── web3.ts                 # Web3 configuration
│   │   ├── api.ts                  # API client setup
│   │   └── utils.ts                # Utility functions
│   ├── providers/
│   │   ├── Providers.tsx           # Combined providers
│   │   ├── Web3Provider.tsx        # Web3 provider setup
│   │   └── TranslationProvider.tsx  # i18n provider
│   ├── store/
│   │   ├── useMenuStore.ts         # Menu state (Zustand)
│   │   ├── useTabStore.ts          # Tab state (Zustand)
│   │   └── useUserStore.ts         # User state (Zustand)
│   ├── styles/
│   │   ├── globals.css             # Global CSS
│   │   └── components.css          # Component styles
│   ├── translations/
│   │   ├── en.json                 # English translations
│   │   └── es.json                 # Spanish translations
│   └── types/
│       ├── auth.ts                 # Auth type definitions
│       ├── user.ts                 # User type definitions
│       └── api.ts                  # API type definitions
├── public/
│   ├── icons/                      # Application icons
│   ├── images/                     # Static images
│   └── favicon.ico                 # Favicon
├── .env.development.sample         # Development environment template
├── .env.production.sample          # Production environment template
├── .gitattributes                  # Git attributes
├── .gitignore                      # Git ignore patterns
├── components.json                 # shadcn/ui configuration
├── next.config.mjs                 # Next.js configuration
├── package.json                    # Dependencies and scripts
├── postcss.config.mjs              # PostCSS configuration
├── tailwind.config.ts              # Tailwind CSS configuration
├── tsconfig.json                   # TypeScript configuration
└── README.md                       # Frontend documentation
```

### Frontend Key Files

- **`src/app/layout.tsx`**: Root layout with providers and global components
- **`src/components/auth/`**: Authentication components using SIWE
- **`src/lib/web3.ts`**: Web3 configuration with Wagmi and Reown/AppKit
- **`src/store/`**: Zustand stores for client-side state management
- **`src/translations/`**: Internationalization files (English/Spanish)

---

## Contracts Structure

```
contracts/
├── .github/
│   └── workflows/
│       └── test.yml            # Contract testing workflow
├── lib/
│   └── forge-std/              # Forge standard library (git submodule)
│       ├── src/                # Standard library source
│       ├── test/               # Standard library tests
│       └── scripts/            # Utility scripts
├── script/
│   └── Counter.s.sol           # Deployment script for Counter
├── src/
│   └── Counter.sol             # Simple counter contract
├── test/
│   └── Counter.t.sol           # Counter contract tests
├── .gitignore                  # Git ignore patterns
├── foundry.toml                # Foundry configuration
└── README.md                   # Contracts documentation
```

### Contracts Key Files

- **`src/Counter.sol`**: Basic counter smart contract example
- **`script/Counter.s.sol`**: Foundry deployment script
- **`test/Counter.t.sol`**: Contract unit tests
- **`foundry.toml`**: Foundry project configuration

---

## Documentation Structure

```
docs/
├── prd/                        # Product Requirements Documents
├── API_DOCUMENTATION.md        # Comprehensive API documentation
├── FOLDER_STRUCTURE.md         # This file - project structure guide
└── [Additional docs to be created]
```

---

## Configuration Files

### Root Level

- **`.env.example`**: Template for environment variables
- **`.env.sample`**: Sample environment configuration
- **`docker-compose.yml`**: Multi-service Docker setup
- **`package.json`**: Root package configuration for monorepo
- **`start.sh`**: Convenience script to start all services

### Backend

- **`go.mod`**: Go module dependencies
- **`Makefile`**: Build, test, and development commands
- **`Dockerfile`**: Container configuration

### Frontend

- **`next.config.mjs`**: Next.js framework configuration
- **`tailwind.config.ts`**: Tailwind CSS styling configuration
- **`tsconfig.json`**: TypeScript compiler configuration
- **`components.json`**: shadcn/ui component library configuration

### Contracts

- **`foundry.toml`**: Foundry development framework configuration

---

## Key Design Patterns

### Backend Architecture

- **Clean Architecture**: Separation of concerns with internal packages
- **Middleware Pattern**: Composable HTTP middleware for cross-cutting concerns
- **Repository Pattern**: Database abstraction in `internal/database/`
- **Handler Pattern**: HTTP request handlers grouped by feature

### Frontend Architecture

- **Component-Based**: Reusable React components with TypeScript
- **Provider Pattern**: Context providers for global state
- **Custom Hooks**: Reusable logic with React hooks
- **Store Pattern**: Zustand for client-side state management

### Smart Contracts

- **Foundry Framework**: Modern Solidity development with testing
- **Script Pattern**: Deployment scripts for contract management
- **Test-Driven**: Comprehensive unit tests for contracts

---

## Development Workflow

### File Organization Principles

1. **Feature-Based Grouping**: Related functionality grouped together
2. **Separation of Concerns**: Clear boundaries between layers
3. **Consistent Naming**: Descriptive names following conventions
4. **Modular Structure**: Reusable and testable components
5. **Configuration Management**: Environment-specific settings

### Testing Structure

- **Backend**: `*_test.go` files alongside source code
- **Frontend**: `__tests__/` directories or `.test.ts` files
- **Contracts**: `test/` directory with `.t.sol` files

### Build Artifacts

- **Backend**: `bin/` directory (gitignored)
- **Frontend**: `.next/` directory (gitignored)
- **Contracts**: `out/` directory (gitignored)

This structure supports a scalable, maintainable Web3 application with clear separation between backend API, frontend interface, and smart contracts.
