# Jule Playground

> An online sandbox to quickly test [Jule](https://jule.dev/) code.

## Setup

You will need [Docker](https://www.docker.com/) and a [Go](https://go.dev) compiler (version >= 1.25.5).

Clang and julec are isolated inside the Dockerfile, which can be built this way:

```
docker build -t jule-clang .
```

### Locally

Type `go run .` then open `0.0.0.0:8080/playground` in your browser.

### On a server

You need to have Docker installed on the server.

There is a [script](./deploy.sh) to easily deploy the project files. Before running it check out the paths and make sure to have a `.env` file containing `SERVER_IP="your_server_ip"`.
