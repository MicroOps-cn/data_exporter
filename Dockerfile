FROM quay.io/prometheus/busybox-linux-amd64:latest
LABEL maintainer="MicroOps"

COPY data_exporter  /bin/data_exporter
COPY examples       /etc/data_exporter/
WORKDIR /etc/data_exporter/
EXPOSE      9116
ENTRYPOINT  [ "/bin/data_exporter" ]
CMD         [ "--config.file=/etc/data_exporter/config.yml" ]