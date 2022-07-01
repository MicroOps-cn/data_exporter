### Try it yourself?
```bash
git clone https://github.com/MicroOps-cn/data_exporter
cd data_exporter/examples/xml_3gpp_ts/
docker run --rm -it -v ${PWD}:/etc/data_exporter/ microops/data_exporter debug --config.path=/etc/data_exporter/
```