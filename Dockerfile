# ---- build stage ----
FROM golang:1.25-alpine AS build
WORKDIR /src

# Copy module manifests first so the dependency-download layer caches
# independently of source changes. Without this split, every code edit
# invalidates the layer cache and re-downloads pgx + goose on every build.
COPY go.mod go.sum ./
RUN go mod download

# Now copy source and build a static binary.
#   CGO_ENABLED=0    no glibc dependency; runs on distroless/static or scratch.
#   -trimpath        strip absolute paths for build reproducibility.
#   -ldflags="-s -w" strip symbol table + DWARF debug info (~25% smaller).
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# ---- run stage ----
# distroless/static contains only a minimal runtime (CA bundle + tzdata + libc).
# No shell, no package manager, no utilities — minimal attack surface, ~2 MB base.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api /api
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/api"]
