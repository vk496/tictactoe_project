# syntax=docker/dockerfile:1

FROM golang:1.26 AS build
WORKDIR /src

# Fetch modules first (cached while go.mod/go.sum are unchanged).
COPY go.mod go.sum ./
RUN go mod download

# Install the code generators. The generated Go + OpenAPI are not committed, so
# the image regenerates them from the protos on every build — no stale artifacts.
RUN go install github.com/bufbuild/buf/cmd/buf@latest \
 && go install google.golang.org/protobuf/cmd/protoc-gen-go@latest \
 && go install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest \
 && go install github.com/sudorandom/protoc-gen-connect-openapi@latest

COPY . .
RUN PATH="$(go env GOPATH)/bin:$PATH" buf generate
RUN CGO_ENABLED=0 go build -o /out/tictactoe .

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/tictactoe /tictactoe
EXPOSE 8080
# The role (lobby|worker|gateway) is passed as the command in docker-compose.
ENTRYPOINT ["/tictactoe"]
