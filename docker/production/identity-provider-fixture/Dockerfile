# syntax=docker/dockerfile:1.7
FROM golang:1.25.0 AS builder
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/identity-provider ./docker/production/identity-provider-fixture

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/identity-provider /identity-provider
USER nonroot:nonroot
ENTRYPOINT ["/identity-provider"]
