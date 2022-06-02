package collector

import "gopkg.in/alecthomas/kingpin.v2"

func AddFlags(flagSet *kingpin.Application) {
	flagSet.Flag("datasource.default-timeout", "Default timeout").Default("30s").DurationVar(&DatasourceDefaultTimeout)
}
