# syntax=docker/dockerfile:1.7

FROM golang:1.25.0 AS builder
WORKDIR /app
COPY . .
# go.mod intentionally replaces github.com/teamgram/proto with the sibling
# checkout. Keep that dependency outside this repository while making it
# available through the BuildKit named context supplied by Compose.
COPY --from=proto . /proto
RUN ./build.sh

FROM ubuntu:latest
# Skip apt install to avoid network issues - ffmpeg/psmisc are optional for core functionality
WORKDIR /app
COPY --from=builder /app/teamgramd/ /app/
RUN chmod +x /app/docker/entrypoint.sh /app/docker/healthcheck.sh
ENTRYPOINT ["/app/docker/entrypoint.sh"]
