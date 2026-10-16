FROM golang:1.25.0 AS builder
WORKDIR /app
COPY . .
RUN ./build.sh

FROM ubuntu:latest
# Skip apt install to avoid network issues - ffmpeg/psmisc are optional for core functionality
WORKDIR /app
COPY --from=builder /app/teamgramd/ /app/
RUN chmod +x /app/docker/entrypoint.sh /app/docker/healthcheck.sh
ENTRYPOINT ["/app/docker/entrypoint.sh"]
