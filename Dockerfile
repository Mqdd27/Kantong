FROM node:24-alpine AS web
WORKDIR /src
COPY package.json package-lock.json ./
COPY apps/web/package.json apps/web/
COPY apps/api/package.json apps/api/
RUN npm ci
COPY apps/web/ apps/web/
RUN npm run build -w web

FROM golang:1.27 AS build
WORKDIR /src
COPY apps/api/go.mod apps/api/go.sum ./
RUN go mod download
COPY apps/api/ ./
COPY --from=web /src/apps/api/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /money-manager . && mkdir /data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /money-manager /money-manager
COPY --from=build --chown=65532:65532 /data /data
ENV TZ=Asia/Jakarta DB_PATH=/data/money.db PORT=8080
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["/money-manager"]
