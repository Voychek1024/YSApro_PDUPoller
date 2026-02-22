# syntax=docker/dockerfile:1

FROM golang:1.25-alpine

# Set destination for COPY
WORKDIR /app

# Download Go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code. Note the slash at the end, as explained in
# https://docs.docker.com/reference/dockerfile/#copy
COPY *.go ./

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -o /poller

# Copy config
COPY config.yaml /config.yaml
EXPOSE 18050

# Forward logs to docker-cli
RUN ln -sf /dev/stdout /var/log/pdu_poller.log

# Run
CMD ["/poller"]
