FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /money-manager . && mkdir /data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /money-manager /money-manager
COPY --from=build --chown=65532:65532 /data /data
ENV TZ=Asia/Jakarta DB_PATH=/data/money.db PORT=8080
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["/money-manager"]
