# ---- build stage ----
FROM golang:1.22-alpine AS build
WORKDIR /src

# No third-party dependencies, so there's no go.sum and no download step.
COPY go.mod ./
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# ---- run stage ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api /api
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/api"]
