### Try it yourself?
```bash
git clone https://github.com/MicroOps-cn/data_exporter
cd data_exporter/examples/spring_actuator/
docker run --rm --name spring-demo -itd microops/spring-demo:1.0.0
docker run --rm -it -v ${PWD}:/etc/data_exporter/ --network container:spring-demo microops/data_exporter debug --config.path=/etc/data_exporter/
```