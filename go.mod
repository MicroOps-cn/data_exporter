module github.com/MicroOps-cn/data_exporter

require (
	github.com/beevik/etree v1.1.0
	github.com/cespare/xxhash/v2 v2.1.1
	github.com/go-kit/log v0.1.0
	github.com/hpcloud/tail v1.0.0
	github.com/prometheus/client_golang v1.11.0
	github.com/prometheus/client_model v0.2.0
	github.com/prometheus/common v0.29.0
	github.com/prometheus/exporter-toolkit v0.6.1
	github.com/stretchr/testify v1.7.0
	github.com/tidwall/gjson v1.9.0
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b
)

require (
	github.com/alecthomas/template v0.0.0-20190718012654-fb15b899a751 // indirect
	github.com/alecthomas/units v0.0.0-20190924025748-f65c72e2690d // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/go-logfmt/logfmt v0.5.0 // indirect
	github.com/golang/protobuf v1.4.3 // indirect
	github.com/jpillora/backoff v1.0.0 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.1 // indirect
	github.com/mwitkow/go-conntrack v0.0.0-20190716064945-2f068394615f // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/procfs v0.6.0 // indirect
	github.com/tidwall/match v1.0.3 // indirect
	github.com/tidwall/pretty v1.1.0 // indirect
	golang.org/x/crypto v0.0.0-20210616213533-5ff15b29337e // indirect
	golang.org/x/net v0.0.0-20210525063256-abc453219eb5 // indirect
	golang.org/x/oauth2 v0.0.0-20210514164344-f6687ab2804c // indirect
	golang.org/x/sys v0.0.0-20210630005230-0f9fa26af87c // indirect
	golang.org/x/text v0.3.6 // indirect
	google.golang.org/appengine v1.6.6 // indirect
	google.golang.org/protobuf v1.26.0-rc.1 // indirect
	gopkg.in/fsnotify.v1 v1.4.7 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)

replace github.com/hpcloud/tail v1.0.0 => ./pkg/tail@v1.0.0

go 1.18
