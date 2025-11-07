################################################################################
# --- Go Project Settings ---
################################################################################

APP_NAME := go-server
GOCMD := go
GOBUILD := $(GOCMD) build
BUILDDIR := bin

# --- Deployment Settings ---
SERVER_USER := ubuntu
SERVER_HOST := your-aws-server-ip-or-domain
SERVER_PATH := /opt/go-server
PEM_KEY := /Users/rr/Documents/cwas.pem

################################################################################
# --- Main Targets ---
################################################################################

all: build

# Build the application for Linux (cross-compilation)
build:
    @echo "Building $(APP_NAME) for linux/amd64..."
    @mkdir -p $(BUILDDIR)
    GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BUILDDIR)/$(APP_NAME) .

# DEPLOYMENT: Copy binary and static files, set permissions, and restart
deploy: build
    @echo "Deploying $(APP_NAME) to $(SERVER_HOST)..."
    # 1. Copy the compiled binary
    scp -i $(PEM_KEY) $(BUILDDIR)/$(APP_NAME) $(SERVER_USER)@$(SERVER_HOST):$(SERVER_PATH)/
    # 2. Copy the static 'templates' directory
    scp -i $(PEM_KEY) -r templates/ $(SERVER_USER)@$(SERVER_HOST):$(SERVER_PATH)/
    @echo "Files copied. Setting permissions and restarting service..."
    # 3. Make the binary executable and set ownership, then restart the service
    ssh -i $(PEM_KEY) $(SERVER_USER)@$(SERVER_HOST) "sudo chmod +x $(SERVER_PATH)/$(APP_NAME) && sudo chown -R gs:gs $(SERVER_PATH) && sudo systemctl restart go-server.service"
    @echo "Deployment complete."

run:
    @echo "Running $(APP_NAME) locally..."
    $(GOCMD) run .

clean:
    @echo "Cleaning..."
    rm -rf $(BUILDDIR)

################################################################################
# --- Remote Management Targets ---
################################################################################

service-status:
    @echo "Checking status of go-server.service..."
    ssh -i $(PEM_KEY) $(SERVER_USER)@$(SERVER_HOST) "sudo systemctl status go-server.service --no-pager"

service-logs:
    @echo "Following logs for go-server.service..."
    ssh -i $(PEM_KEY) $(SERVER_USER)@$(SERVER_HOST) "sudo journalctl -u go-server.service -f"

################################################################################
# --- Phony Targets ---
################################################################################

.PHONY: all build deploy run clean service-status service-logs