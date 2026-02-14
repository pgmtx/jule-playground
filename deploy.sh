#!/usr/bin/env bash

if [ ! -f ./.env ]; then
    echo "Please create a .env file with the server IP"
    exit 1
fi

source .env
CGO_ENABLED=0 go build -o main -ldflags "-s -w" .
ssh root@$SERVER_IP "systemctl stop playground"
scp ./main root@$SERVER_IP:/opt/playground/
scp -r ./public root@$SERVER_IP:/opt/playground/
ssh root@$SERVER_IP "systemctl start playground"
echo "Code deployed successfully!"
