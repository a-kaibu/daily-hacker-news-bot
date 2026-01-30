# syntax=docker/dockerfile:1
# check=error=true

ARG GO_VERSION=1.25@sha256:6cc2338c038bc20f96ab32848da2b5c0641bb9bb5363f2c33e9b7c8838f9a208
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION} AS build

WORKDIR /app

ARG TARGETARCH

RUN --mount=type=cache,target=/go/pkg/mod/ \
    --mount=type=bind,target=. \
    CGO_ENABLED=0 GOARCH=$TARGETARCH go build -ldflags="-s" -trimpath -o /bin/server main.go

# ============================================================= #
FROM gcr.io/distroless/static-debian13:nonroot@sha256:b5b9fd04c8dcf72a173183c0b7dee47e053e002246b308a59f3441db7b8b9cc4 AS final

COPY --from=build /bin/server /bin/

WORKDIR /app

ENV TZ=Asia/Tokyo

ENTRYPOINT ["/bin/server"]

