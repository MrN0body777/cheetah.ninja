# Cheetah Chat

![Go Version](https://img.shields.io/badge/Go-1.20+-00ADD8?style=flat-square&logo=go)
![License](https://img.shields.io/badge/License-MIT-green.svg)

A high-performance, secure, and scalable real-time chat application built with Go and native WebSockets. This project demonstrates a modern backend architecture focused on concurrency, security, and web performance best practices.

---

## Table of Contents

- [Cheetah Chat](#cheetah-chat)
  - [Table of Contents](#table-of-contents)
  - [Why Go?](#why-go)
  - [How It Works](#how-it-works)
  - [Getting Started](#getting-started)
  - [Configuration](#configuration)
  - [Production Deployment](#production-deployment)
  - [License](#license)
  - [Acknowledgments](#acknowledgments)

---

## Why Go?

Go was chosen for this project due to its exceptional support for concurrency. Its lightweight goroutines and channels are a perfect fit for managing thousands of simultaneous WebSocket connections efficiently, making it an ideal choice for building high-throughput, real-time services.

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
    cd cheatah.ninja
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

> **Security Note:** For production, these keys must be long, cryptographically random, and stored securely using your hosting provider's secret management system (e.g., AWS Secrets Manager, HashiCorp Vault).

---

## Production Deployment

This application is designed for production deployment. Key considerations include:

-   **HTTPS:** The application must be served behind a reverse proxy (e.g., Nginx, Caddy) that handles SSL/TLS termination. The application is configured to serve secure WebSocket (`wss://`) connections when it detects an `https` origin.
-   **Process Management:** Use a process manager like `systemd` or `supervisor` to ensure the application runs continuously and restarts automatically on failure.
-   **Graceful Shutdown:** For zero-downtime deployments, the application should implement a handler for `SIGTERM` and `SIGINT` signals to close connections gracefully before shutting down.
-   **State Persistence:** The current implementation uses in-memory state. For horizontal scaling and persistence, integrating a fast in-memory database like **Redis** is the recommended next step.

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
