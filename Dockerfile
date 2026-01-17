# build stage
FROM golang:1.24-alpine AS builder

# install git and ca-certificates (needed for fetching dependencies)
RUN apk add --no-cache git ca-certificates

# set working directory
WORKDIR /app

# copy go mod files first for better caching
COPY go.mod go.sum ./

# download dependencies
RUN go mod download

# copy source code
COPY . .

# build the application
# use cgo_enabled=0 to create a static binary
# use -ldflags to strip debug info and reduce binary size
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o nak .

# runtime stage
FROM alpine:latest

# install ca-certificates for https requests (needed for relay connections)
RUN apk --no-cache add ca-certificates

# create a non-root user
RUN adduser -D -s /bin/sh nakuser

# set working directory
WORKDIR /home/nakuser

# copy the binary from builder stage
COPY --from=builder /app/nak /usr/local/bin/nak

# make sure the binary is executable
RUN chmod +x /usr/local/bin/nak

# switch to non-root user
USER nakuser

# set the entrypoint
ENTRYPOINT ["nak"]

# default command (show help)
CMD ["--help"]
