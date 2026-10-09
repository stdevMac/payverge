# Payverge Authentication System

## Overview

Payverge now supports multiple authentication methods to lower the barrier for user onboarding:

1. **Email/Password** - Traditional registration and login
2. **Google OAuth** - One-click Google sign-in
3. **Instagram OAuth** - Social login via Instagram
4. **Wallet Linking** - Optional feature to connect crypto wallet after signup

## Architecture

### Authentication Flow

```
┌─────────────────────────────────────────────────────────────┐
│                    User Onboarding                          │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│   ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│   │    Email     │  │    Google    │  │  Instagram   │     │
│   │   Password   │  │    OAuth     │  │    OAuth     │     │
│   └──────┬───────┘  └──────┬───────┘  └──────┬───────┘     │
│          │                 │                 │              │
│          └─────────────────┼─────────────────┘              │
│                            ▼                                │
│                   ┌────────────────┐                        │
│                   │  User Account  │                        │
│                   │   (Created)    │                        │
│                   └────────┬───────┘                        │
│                            │                                │
│                            ▼                                │
│                   ┌────────────────┐                        │
│                   │ Optional:      │                        │
│                   │ Link Wallet    │◄─── For crypto payments│
│                   └────────────────┘                        │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### Database Models

#### User Table (Updated)
- `email` - Primary identifier (unique index)
- `address` - Optional wallet address (can be linked later)
- `name` - User's display name
- `auth_method` - Primary auth method used (email, google, wallet)
- `google_id` - Google OAuth user ID
- `email_verified` - Whether email is verified

#### UserAuth Table (New)
- Stores authentication credentials for each provider
- Supports multiple auth methods per user
- Stores password hashes for email auth
- Stores OAuth tokens for social logins
- Stores wallet addresses for linked wallets

## Environment Variables

### Required for Email/Password Auth
```env
# JWT Secret (already exists)
JWT_SECRET=your-secret-key
```

### Required for Google OAuth
```env
GOOGLE_CLIENT_ID=your-google-client-id
GOOGLE_CLIENT_SECRET=your-google-client-secret
```

To get Google OAuth credentials:
1. Go to [Google Cloud Console](https://console.cloud.google.com/)
2. Create a new project or select existing
3. Enable "Google+ API" and "Google Identity"
4. Go to "Credentials" → "Create Credentials" → "OAuth 2.0 Client ID"
5. Set authorized redirect URI: `https://your-domain.com/api/v1/auth/google/callback`

### Required for Instagram OAuth
```env
INSTAGRAM_CLIENT_ID=your-instagram-client-id
INSTAGRAM_CLIENT_SECRET=your-instagram-client-secret
```

To get Instagram OAuth credentials:
1. Go to [Meta for Developers](https://developers.facebook.com/)
2. Create a new app or select existing
3. Add "Instagram Basic Display" product
4. Configure OAuth settings
5. Set redirect URI: `https://your-domain.com/api/v1/auth/instagram/callback`

### Application Base URL
```env
APP_BASE_URL=https://your-domain.com
```

## API Endpoints

### Public Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/v1/auth/register` | Register with email/password |
| POST | `/api/v1/auth/login` | Login with email/password |
| POST | `/api/v1/auth/logout` | Logout |
| GET | `/api/v1/auth/google` | Get Google OAuth URL |
| GET | `/api/v1/auth/google/callback` | Google OAuth callback |
| GET | `/api/v1/auth/instagram` | Get Instagram OAuth URL |
| GET | `/api/v1/auth/instagram/callback` | Instagram OAuth callback |
| POST | `/api/v1/auth/password/reset-request` | Request password reset |
| POST | `/api/v1/auth/password/reset` | Reset password with token |
| POST | `/api/v1/auth/email/verify` | Verify email with token |

### Protected Endpoints (Require Authentication)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v1/auth/me` | Get current user |
| GET | `/api/v1/auth/methods` | Get linked auth methods |
| POST | `/api/v1/auth/wallet/link` | Link wallet to account |
| DELETE | `/api/v1/auth/wallet/unlink` | Unlink wallet from account |

## Frontend Components

### HybridAuthProvider
Located at: `src/providers/HybridAuthProvider.tsx`

Provides authentication context with:
- `user` - Current user data
- `isLoading` - Loading state
- `isAuthenticated` - Whether user is logged in
- `login(email, password)` - Email/password login
- `register(email, password, name)` - Email/password registration
- `logout()` - Logout
- `loginWithGoogle()` - Initiate Google OAuth
- `loginWithInstagram()` - Initiate Instagram OAuth
- `linkWallet(address, message, signature)` - Link wallet
- `unlinkWallet()` - Unlink wallet

### Pages
- `/login` - Login page with email/password and OAuth options
- `/register` - Registration page
- `/forgot-password` - Password reset request

### Components
- `WalletLinkCard` - Component for linking/unlinking wallet in user settings

## Migration from Wallet-Only Auth

Existing users who signed up with wallets are automatically migrated:
1. Their wallet address is preserved
2. A `UserAuth` record is created with provider="wallet"
3. They can continue using wallet auth
4. They can optionally add email/OAuth methods

## Security Considerations

1. **Password Hashing**: Uses bcrypt with default cost
2. **JWT Tokens**: 24-hour expiry, signed with HS256
3. **OAuth State**: Random state tokens prevent CSRF
4. **Email Verification**: Optional but recommended
5. **Password Reset**: Tokens expire after 1 hour

## Usage Examples

### Frontend: Login with Email
```typescript
import { useAuth } from "@/providers/HybridAuthProvider";

function LoginComponent() {
  const { login } = useAuth();
  
  const handleLogin = async () => {
    try {
      await login("user@example.com", "password123");
      // Redirect to dashboard
    } catch (error) {
      // Handle error
    }
  };
}
```

### Frontend: Link Wallet
```typescript
import { useAuth } from "@/providers/HybridAuthProvider";
import { useSignMessage } from "wagmi";

function WalletLink() {
  const { linkWallet } = useAuth();
  const { signMessageAsync } = useSignMessage();
  
  const handleLink = async (address: string) => {
    const message = `Link wallet to Payverge\nAddress: ${address}`;
    const signature = await signMessageAsync({ message });
    await linkWallet(address, message, signature);
  };
}
```

## Backward Compatibility

The legacy wallet-based authentication (SIWE) routes are preserved:
- `/api/v1/auth/challenge`
- `/api/v1/auth/signin`
- `/api/v1/auth/signout`

These continue to work for existing integrations.
