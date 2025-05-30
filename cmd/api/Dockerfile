FROM golang:1.21 as builder

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . .

RUN go build -o ansible-api

# Final image with Ansible
FROM ubuntu:22.04

# Install ansible and dependencies
RUN apt-get update && \
    apt-get install -y ansible curl ca-certificates && \
    apt-get clean

COPY --from=builder /app/ansible-api /usr/local/bin/ansible-api

# Set working dir for playbooks
WORKDIR /opt/playbooks

# Expose API port
EXPOSE 8080

CMD ["ansible-api"]