# ---- build stage ----
FROM golang:1.22-alpine AS build
WORKDIR /src

# This project has no third-party dependencies, so there is no go.sum and no
# module download step. Copying go.mod first still lets Docker cache the
# (empty) dependency graph layer.
COPY go.mod ./
COPY . .

# Static, stripped binary so it runs in a distroless/scratch image.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# ---- run stage ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api /api
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/api"]
