# Jule Playground

> An online sandbox to quickly test Jule code.

## Setup

### Locally

You will need [Docker](https://www.docker.com/) and a [Go](https://go.dev) compiler.

```
docker build -t jule-clang .
go run .
```

Then open `0.0.0.0:8080` in your browser.

### On a server

You need to define a systemd service and check paths inside the script `deploy.sh`.

After that to create a `.env` file containing `SERVER_IP="your_server_ip"`, then you can execute `deploy.sh`.
