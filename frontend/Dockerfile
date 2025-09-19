# Use an official Node runtime as the base image
FROM node:20.13.1-alpine AS deps

# Install dependencies only when needed
WORKDIR /app

# Copy package files
COPY package.json package-lock.json* ./

# Install dependencies with minimal footprint
RUN apk add --no-cache --virtual .build-deps \
    python3 \
    make \
    g++ \
    linux-headers && \
    npm ci --only=production && \
    apk del .build-deps && \
    npm cache clean --force

# Builder stage
FROM node:20.13.1-alpine AS builder

# Add build arguments
ARG NODE_ENV=production
ARG ENV_COPY=development

# Set environment variables
ENV NODE_ENV=$NODE_ENV
ENV ENV_COPY=$ENV_COPY
ENV NEXT_TELEMETRY_DISABLED=1
ARG NEXT_PUBLIC_MAINTENANCE_MODE
ENV NEXT_PUBLIC_MAINTENANCE_MODE=$NEXT_PUBLIC_MAINTENANCE_MODE
ARG NEXT_PUBLIC_COMING_SOON_MODE
ENV NEXT_PUBLIC_COMING_SOON_MODE=$NEXT_PUBLIC_COMING_SOON_MODE
ARG NEXT_PUBLIC_PROJECT_ID
ENV NEXT_PUBLIC_PROJECT_ID=$NEXT_PUBLIC_PROJECT_ID
ARG NEXT_PUBLIC_API_URL
ENV NEXT_PUBLIC_API_URL=$NEXT_PUBLIC_API_URL

WORKDIR /app

# Copy package files and install ALL dependencies (including dev)
COPY package.json package-lock.json* ./
RUN apk add --no-cache --virtual .build-deps \
    python3 \
    make \
    g++ \
    linux-headers && \
    npm ci && \
    apk del .build-deps

# Copy source code (use .dockerignore to exclude unnecessary files)
COPY . .

# Build the application
RUN npm run build

# Production image
FROM node:20.13.1-alpine AS runner

# Create non-root user for security
RUN addgroup --system --gid 1001 nodejs && \
    adduser --system --uid 1001 nextjs

# Set working directory
WORKDIR /app

# Set environment to production
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1

# Copy production dependencies from deps stage
COPY --from=deps --chown=nextjs:nodejs /app/node_modules ./node_modules

# Copy built application from builder stage
COPY --from=builder --chown=nextjs:nodejs /app/public ./public
COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/.next/static ./.next/static

# Switch to non-root user
USER nextjs

# Expose port
EXPOSE 3000

ENV PORT 3000
ENV HOSTNAME "0.0.0.0"

# Start the application
CMD ["node", "server.js"]
