################################################################################
# --- Go Project Settings ---
################################################################################

APP_NAME := go-server
GOCMD := go
GOBUILD := $(GOCMD) build
BUILDDIR := build

# --- Deployment Settings ---
SERVER_USER := ubuntu
SERVER_HOST := ec2-51-21-51-104.eu-north-1.compute.amazonaws.com
SERVER_PATH := /home/ubuntu/go-server

PEM_KEY := /Users/rr/Documents/cwas.pem

################################################################################
# --- Main Targets ---
################################################################################

all: build

build:
    @echo "Building $(APP_NAME) for current platform..."
    $(GOBUILD) -o $(BUILDDIR)/$(APP_NAME) .

# DEPLOYMENT: Copy source to server, then build and restart the service there
deploy:
    @echo "Deploying source to $(SERVER_HOST)..."
    rsync -avz --progress -e "ssh -i $(PEM_KEY)" \
        --exclude='$(BUILDDIR)' \
        --exclude='.git/' \
        --exclude='.DS_Store' \
        ./ $(SERVER_USER)@$(SERVER_HOST):$(SERVER_PATH)
    @echo "Source deployed. Building and restarting service on server..."
    ssh -i $(PEM_KEY) $(SERVER_USER)@$(SERVER_HOST) "cd $(SERVER_PATH) && make && sudo systemctl restart go-server.service"
    @echo "Deployment complete."

run:
    @echo "Running $(APP_NAME)..."
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