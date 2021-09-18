ARG ARCH="amd64"
ARG OS="linux"
FROM quay.io/prometheus/busybox-${OS}-${ARCH}:latest
LABEL maintainer="MicroOps"

ARG ARCH="amd64"
ARG OS="linux"
COPY .build/${OS}-${ARCH}/data_exporter  /bin/data_exporter
COPY blackbox.yml       /etc/data_exporter/config.yml

EXPOSE      9116
ENTRYPOINT  [ "/bin/data_exporter" ]
CMD         [ "--config.file=/etc/data_exporter/config.yml" ]