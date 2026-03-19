FROM golang:alpine AS build

ARG VERSION="unknown-docker"

WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X github.com/Scrin/RuuviBridge/common/version.Version=${VERSION}" \
    -o /ruuvibridge ./cmd/...

FROM alpine

COPY --from=build /ruuvibridge /usr/local/bin/ruuvibridge

USER 1337:1337

CMD ["ruuvibridge"]
