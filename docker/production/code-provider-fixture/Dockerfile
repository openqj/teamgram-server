# syntax=docker/dockerfile:1.7
FROM golang:1.25.0 AS builder
WORKDIR /src
COPY main.go .
RUN CGO_ENABLED=0 go build -trimpath -o /out/code-provider main.go

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/code-provider /code-provider
USER nonroot:nonroot
ENTRYPOINT ["/code-provider"]
