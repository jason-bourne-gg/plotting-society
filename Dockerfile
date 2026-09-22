# Two stages: build with the Go toolchain, ship a ~15 MB scratch image.
# A small image is not vanity here — it is why cold starts on a scale-to-zero
# free tier stay under half a second.

FROM golang:1.23-alpine AS build

WORKDIR /src

# Dependencies first, so a code-only change reuses this layer.
COPY go.mod go.sum* ./
RUN go mod download

COPY . .

# CGO off gives a fully static binary; trimpath and -w -s drop debug weight.
RUN CGO_ENABLED=0 GOOS=linux go build \
        -trimpath -ldflags="-w -s" \
        -o /out/api ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux go build \
        -trimpath -ldflags="-w -s" \
        -o /out/seed ./cmd/seed

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/api /api
COPY --from=build /out/seed /seed

USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/api"]
