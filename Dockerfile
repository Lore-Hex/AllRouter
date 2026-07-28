# syntax=docker/dockerfile:1

FROM golang:1.23 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/allrouter ./cmd/allrouter

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/allrouter /allrouter
USER nonroot:nonroot
EXPOSE 8383
ENTRYPOINT ["/allrouter"]
