FROM docker.io/library/golang:1.27.0-bookworm AS source
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

FROM source AS test
RUN make check

FROM source AS build
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_TIME=unknown
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildTime=${BUILD_TIME}" -o /out/zte-c300-monitoring ./cmd/api

# The fixture is a test tool and is never included in the runtime image.
FROM source AS fixture-build
RUN CGO_ENABLED=0 go build -trimpath -o /out/snmp-fixture ./test/snmp-fixture

FROM gcr.io/distroless/static-debian12:nonroot AS fixture
COPY --from=fixture-build /out/snmp-fixture /snmp-fixture
USER nonroot:nonroot
ENTRYPOINT ["/snmp-fixture"]

FROM gcr.io/distroless/static-debian12:nonroot AS runtime
ENV APP_ENV=production SERVER_HOST=0.0.0.0 SERVER_PORT=8081
COPY --from=build /out/zte-c300-monitoring /zte-c300-monitoring
COPY LICENSE /LICENSE
EXPOSE 8081
USER nonroot:nonroot
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD ["/zte-c300-monitoring", "healthcheck"]
ENTRYPOINT ["/zte-c300-monitoring"]
