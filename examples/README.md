### Log file content
```
$ cat /var/log/nginx/ops.access.log
127.0.0.1 - - [15/Apr/2022:15:10:30 +0800] "GET /admin/ HTTP/1.1" 404 555 "-" "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/99.0.4844.84 Safari/537.36" "-"
127.0.0.1 - - [15/Apr/2022:15:10:45 +0800] "POST /admin/ HTTP/1.1" 404 153 "-" "curl/7.64.0" "-"
127.0.0.1 - - [15/Apr/2022:15:14:53 +0800] "GET / HTTP/1.1" 404 555 "-" "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/99.0.4844.84 Safari/537.36" "-"
```

### Config file
```yaml
collects:
  - name: "nginx"
    data_format: "regex"
    datasource:
      - type: "file"
        url: "/var/log/nginx/ops.access.log"
        read_mode: "stream"
    metrics:
      - name: "nginx_response_status_count"
        metric_type: "counter"
        match:
          datapoint: '^[\d\.]+ - - \[\S+ \S+\] "(?P<method>\S+) \S+ (?P<protocol>\S+)" (?P<status>\d+) .*$'
      - name: "nginx_request_count"
        metric_type: "counter"
      - name: "nginx_response_bytes_count"
        metric_type: "counter"
        match:
          datapoint: '[\d\.]+ - - \[\S+ \S+\] "\S+ \S+ \S+" \d+ (?P<__value__>\d+) .*'
```

### metrics
```bash
$ curl http://localhost:9116/nginx/metrics
# HELP nginx_request_count 
# TYPE nginx_request_count counter
nginx_request_count 3
# HELP nginx_response_bytes_count 
# TYPE nginx_response_bytes_count counter
nginx_response_bytes_count 1263
# HELP nginx_response_status_count 
# TYPE nginx_response_status_count counter
nginx_response_status_count{method="GET",protocol="HTTP/1.1",status="404"} 2
nginx_response_status_count{method="POST",protocol="HTTP/1.1",status="404"} 1
# HELP promhttp_metric_handler_errors_total Total number of internal errors encountered by the promhttp metric handler.
# TYPE promhttp_metric_handler_errors_total counter
promhttp_metric_handler_errors_total{cause="encoding"} 0
promhttp_metric_handler_errors_total{cause="gathering"} 0
```