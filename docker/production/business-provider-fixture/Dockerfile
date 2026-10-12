# syntax=docker/dockerfile:1.7
FROM golang:1.25.0 AS builder
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/business-provider ./docker/production/business-provider-fixture

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/business-provider /business-provider
USER nonroot:nonroot
ENTRYPOINT ["/business-provider"]
