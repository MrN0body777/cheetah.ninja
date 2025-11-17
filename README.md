

Of course! Here is a complete, professional, and generic README that incorporates your setup (Nginx, Let's Encrypt, systemd) but is written with best practices and placeholders so anyone can follow it.

This version emphasizes creating a dedicated system user and compiling the application for production.

---

# Cheetah Chat

<p align="center">
    <img src="https://img.shields.io/badge/Go-1.20+-00ADD8?style=flat-square&logo=go" alt="Go Version">
    <img src="https://img.shields.io/badge/License-MIT-green.svg" alt="License">
</p>

<p align="center">
A high-performance, secure, and scalable real-time chat application built with Go and native WebSockets.
</p>

---

## Table of Contents

- [How It Works](#how-it-works)
- [Getting Started](#getting-started)
- [Configuration](#configuration)
- [Production Deployment](#production-deployment)
- [License](#license)
- [Acknowledgments](#acknowledgments)

---

## How It Works

The application follows a simple but robust architecture:

1.  **HTTP Server:** A standard Go HTTP server serves the initial HTML pages.
2.  **WebSocket Upgrade:** When a client connects, the server "upgrades" the HTTP connection to a persistent WebSocket connection.
3.  **Token-Based Authentication:** The server issues a short-lived JSON Web Token (JWT) upon page load. The client must present this valid token to establish a WebSocket connection, ensuring all participants are authenticated.
4.  **In-Memory State:** Each chat room and its list of connected clients are managed in-memory. Mutexes are used to prevent race conditions when modifying shared state.
5.  **Message Broadcasting:** When a message is received, it is broadcast to all connected clients in the same room in real-time.
6.  **Performance:** A middleware layer automatically compresses HTTP responses using Brotli (preferred) or Gzip, significantly reducing page load times for users.

---

## Getting Started

To run a copy of the project locally, ensure you have **Go 1.20+** installed and follow these steps.

1.  **Clone the repository:**
    ```bash
    git clone https://github.com/MrN0body777/cheetah.ninja.git
    cd cheetah.ninja
    ```

2.  **Install dependencies:**
    ```bash
    go mod tidy
    ```

3.  **Set environment variables:**
    The application requires three environment variables for cryptographic operations. For local development, you can set them in your terminal.

    **On macOS / Linux:**
    ```bash
    export HASH_KEY="your-32-byte-long-hash-key-here-"
    export BLOCK_KEY="your-32-byte-long-block-key-here-"
    export JWT_SECRET="your-super-secret-jwt-signing-key"
    ```

    **On Windows (Command Prompt):**
    ```cmd
    set HASH_KEY="your-32-byte-long-hash-key-here-"
    set BLOCK_KEY="your-32-byte-long-block-key-here-"
    set JWT_SECRET="your-super-secret-jwt-signing-key"
    ```

4.  **Run the application:**
    ```bash
    go run main.go
    ```

    The server will start on port 8080 by default. Access the application at `http://localhost:8080`.

---

## Configuration

The application is configured via environment variables, allowing for flexible deployment across different environments.

| Variable     | Description                                                                 |
|--------------|-----------------------------------------------------------------------------|
| `PORT`       | The port for the server to listen on. Defaults to `8080`.              |
| `HASH_KEY`   | A 32-byte key used by `gorilla/securecookie` for authentication.   |
| `BLOCK_KEY`  | A 32-byte key used by `gorilla/securecookie` for encryption.      |
| `JWT_SECRET` | The secret key used to sign and verify JSON Web Tokens.               |

> ** Security Note:** For production, these keys must be long, cryptographically random, and stored securely using your hosting provider's secret management system (e.g., AWS Secrets Manager, HashiCorp Vault).

---

## Production Deployment

This guide covers a robust production deployment on a Linux server (like AWS EC2) using Nginx as a reverse proxy, Let's Encrypt for SSL, and `systemd` to manage the application as a service.

### 1. Create a Dedicated System User

For security, the application should run as a non-root user with limited permissions.

```bash
# Creates a system user and group named 'cheetahchat' without a login shell
sudo adduser --system --group cheetahchat
```

### 2. Compile and Deploy the Application

For production, compile the application into a binary for faster startup and better performance.

On your local machine (or any build machine), run:
```bash
# Build for a standard 64-bit Linux server
GOOS=linux GOARCH=amd64 go build -o cheetah-chat main.go
```

Now, upload the `cheetah-chat` binary to your server and place it in a standard location like `/opt`.

```bash
# On your server, create the directory
sudo mkdir /opt/cheetahchat

# Copy the binary to the server (using scp as an example)
scp cheetah-chat your_user@your_server_ip:/tmp/

# On the server, move the binary and set permissions
sudo mv /tmp/cheetah-chat /opt/cheetahchat/
sudo chown cheetahchat:cheetahchat /opt/cheetahchat/cheetah-chat
sudo chmod +x /opt/cheetahchat/cheetah-chat
```

### 3. Set up Let's Encrypt with Certbot

Install Certbot to obtain a free SSL certificate. On Ubuntu/Debian:

```bash
sudo apt update
sudo apt install certbot python3-certbot-nginx
```

Obtain a certificate for your domain. Certbot will automatically configure Nginx for you.

```bash
sudo certbot --nginx -d your_domain.com -d www.your_domain.com
```

### 4. Configure Nginx

Certbot's configuration is good, but ensure your `/etc/nginx/sites-available/your_domain.com` file has a specific location block for WebSockets.

```nginx
# Redirect HTTP to HTTPS
server {
    listen 80;
    server_name your_domain.com www.your_domain.com;
    return 301 https://$server_name$request_uri;
}

# Main server block for HTTPS
server {
    listen 443 ssl http2;
    server_name your_domain.com www.your_domain.com;

    # --- Let's Encrypt SSL Certificates ---
    # These paths are standard for Certbot.
    ssl_certificate /etc/letsencrypt/live/your_domain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/your_domain.com/privkey.pem;

    # --- Modern SSL Configuration (Mozilla Guideline) ---
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384;
    ssl_prefer_server_ciphers off;
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 1d;

    # --- Proxy for Go Application ---
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # --- WebSocket Proxy ---
    # This specific block is required for WebSocket connections to work.
    location /ws {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

Test and reload Nginx:
```bash
sudo nginx -t
sudo systemctl reload nginx
```

### 5. Create and Enable the systemd Service

Create a service file to manage the application:
```bash
sudo nano /etc/systemd/system/cheetah-chat.service
```

Paste the following configuration. This tells `systemd` to run your compiled binary as the `cheetahchat` user.

```ini
[Unit]
Description=Cheetah Chat Service
After=network.target

[Service]
Type=simple
User=cheetahchat
Group=cheetahchat
WorkingDirectory=/opt/cheetahchat

# Use the compiled binary for production
ExecStart=/opt/cheetahchat/cheetah-chat

Restart=on-failure

# --- IMPORTANT: Set your production secrets here ---
Environment=PORT=8080
Environment=HASH_KEY="your-32-byte-long-hash-key-here-"
Environment=BLOCK_KEY="your-32-byte-long-block-key-here-"
Environment=JWT_SECRET="your-super-secret-jwt-signing-key"

[Install]
WantedBy=multi-user.target
```

Finally, enable and start the service:
```bash
sudo systemctl enable cheetah-chat.service
sudo systemctl start cheetah-chat.service
```

You can check its status with `sudo systemctl status cheetah-chat.service`.

---

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.

---

## Acknowledgments

Built with the help of several outstanding open-source libraries:

-   [gorilla/websocket](https://github.com/gorilla/websocket)
-   [golang-jwt/jwt](https://github.com/golang-jwt/jwt)
-   [gorilla/securecookie](https://github.com/gorilla/securecookie)
-   [andybalholm/brotli](https://github.com/andybalholm/brotli)
