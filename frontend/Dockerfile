# Use an official Node runtime as the base image
FROM node:20.13.1-alpine AS builder

# Add build arguments
ARG NODE_ENV=development
ARG ENV_COPY=development

# Set environment variables
ENV NODE_ENV=$NODE_ENV
ENV ENV_COPY=$ENV_COPY
ENV NEXT_TELEMETRY_DISABLED=1
ARG NEXT_PUBLIC_MAINTENANCE_MODE
ENV NEXT_PUBLIC_MAINTENANCE_MODE=$NEXT_PUBLIC_MAINTENANCE_MODE
ARG NEXT_PUBLIC_COMING_SOON_MODE
ENV NEXT_PUBLIC_COMING_SOON_MODE=$NEXT_PUBLIC_COMING_SOON_MODE

# Install build dependencies
RUN apk add --no-cache \
    python3 \
    make \
    g++ \
    linux-headers

# Set the working directory
WORKDIR /app
# COPY .env.${ENV_COPY}.sample .env
# Copy all files to the working directory
COPY . .

# Install all dependencies, including dev dependencies
RUN npm install

# Ensure ESLint is installed and available
RUN npm install --save-dev eslint@^8.0.0 eslint-config-next@14.2.3

# Build the application
RUN npm run build

# Production image
FROM node:20.13.1-alpine AS runner

# Set environment variables
ARG NODE_ENV
ENV NODE_ENV=$NODE_ENV

# Set working directory
WORKDIR /app

# Install sharp and its dependencies
COPY --from=builder /app/package*.json ./
RUN apk add --no-cache \
    python3 \
    make \
    g++ \
    linux-headers && \
    npm install --omit=dev sharp && \
    apk del python3 make g++ linux-headers

# Copy built application
COPY --from=builder /app/public ./public
COPY --from=builder /app/.next/standalone ./
COPY --from=builder /app/.next/static ./.next/static

# Expose port
EXPOSE 3000

# Start the application
CMD ["node", "server.js"]
