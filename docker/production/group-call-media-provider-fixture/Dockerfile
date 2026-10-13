# syntax=docker/dockerfile:1.7
FROM golang:1.25.0 AS builder
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/group-call-media-provider ./docker/production/group-call-media-provider-fixture

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/group-call-media-provider /group-call-media-provider
USER nonroot:nonroot
ENTRYPOINT ["/group-call-media-provider"]
