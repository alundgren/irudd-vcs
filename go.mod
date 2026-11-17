module gitea.dev

go 1.27.3

require (
	gitea.com/go-chi/binding v0.0.0-20260414111559-654cea7ac60a
	gitea.com/go-chi/cache v0.2.1
	gitea.com/lunny/levelqueue v0.5.1
	github.com/chi-middleware/proxy v1.1.1
	github.com/dustin/go-humanize v1.1.0
	github.com/editorconfig/editorconfig-core-go/v2 v2.6.5
	github.com/felixge/fgprof v0.9.5
	github.com/getkin/kin-openapi v0.153.0
	github.com/go-chi/chi/v5 v5.5.0
	github.com/go-chi/cors v1.2.2
	github.com/gogs/chardet v0.0.0-20211120154057-b7413eaefb8f
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/google/pprof v0.0.0-20261111200114-fac431ef4d26
	github.com/google/uuid v1.6.0
	github.com/hashicorp/go-version v1.9.0
	github.com/hashicorp/golang-lru/v2 v2.0.7
	github.com/huandu/xstrings v1.6.3
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/klauspost/compress v1.20.4
	github.com/mattn/go-isatty v0.0.24
	github.com/stretchr/testify v1.12.1
	github.com/syndtr/goleveldb v1.0.0
	github.com/urfave/cli/v3 v3.14.0
	go.yaml.in/yaml/v4 v4.0.0
	golang.org/x/crypto v0.59.0
	golang.org/x/net v0.62.0
	golang.org/x/sync v0.25.0
	golang.org/x/sys v0.50.0
	golang.org/x/text v0.44.0
	gopkg.in/ini.v1 v1.68.0
	modernc.org/sqlite v1.62.0
	xorm.io/builder v0.3.13
	xorm.io/xorm v1.4.3
)

require (
	github.com/dlclark/regexp2 v1.12.0 // indirect
	github.com/fsnotify/fsnotify v1.10.1 // indirect
	github.com/go-openapi/jsonpointer v0.23.1 // indirect
	github.com/go-openapi/swag/jsonname v0.26.1 // indirect
	github.com/go-sql-driver/mysql v1.10.0 // indirect
	github.com/goccy/go-json v0.10.6 // indirect
	github.com/golang/snappy v1.0.0 // indirect
	github.com/mattn/go-sqlite3 v1.14.47 // indirect
	github.com/ncruces/go-strftime v1.1.0 // indirect
	github.com/onsi/ginkgo v1.16.5 // indirect
	github.com/onsi/gomega v1.34.1 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/exp v0.0.0-20260410095643-746e56fc9e2f // indirect
	golang.org/x/mod v0.42.0 // indirect
	modernc.org/libc v1.80.1 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
)

ignore (
	./.venv
	./node_modules
)
