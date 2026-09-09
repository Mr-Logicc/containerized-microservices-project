# ---- build stage ----
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/app .

# ---- final stage ----
# distroless has no shell, no package manager, nothing but the binary and its
# runtime deps -- smaller attack surface, fewer CVEs for ECR's image scan to find.
FROM gcr.io/distroless/static-debian12
COPY --from=build /out/app /app
EXPOSE 8080
ENTRYPOINT ["/app"]
