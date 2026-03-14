# ---- Build stage ----
FROM node:22-alpine AS build

WORKDIR /app

# Copy package files
COPY package*.json ./
RUN npm ci

# Copy source code
COPY . .

# Build the app (production mode by default)
RUN npm run build

# ---- Runtime stage ----
FROM nginx:alpine

# Copy built static files to nginx html directory
COPY --from=build /app/dist /usr/share/nginx/html

# Copy nginx configuration
COPY nginx.conf /etc/nginx/conf.d/default.conf

# Expose port 80
EXPOSE 80

# Start nginx
CMD ["nginx", "-g", "daemon off;"]
