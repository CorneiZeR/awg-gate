module github.com/CorneiZeR/awg-gate

go 1.27.0

toolchain go1.27.4

require (
	go.yaml.in/yaml/v3 v3.0.6
	rsc.io/qr v0.2.0
)

require (
	github.com/amnezia-vpn/amneziawg-go/v3 v3.1.20261116 // indirect
	golang.org/x/crypto v0.59.0 // indirect
	golang.org/x/mod v0.43.0 // indirect
	golang.org/x/net v0.63.0 // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
	golang.org/x/telemetry v0.0.0-20261116191925-e77a43154acb // indirect
	golang.org/x/tools v0.53.0 // indirect
	golang.org/x/vuln v1.10.0 // indirect
	golang.zx2c4.com/wintun v0.0.0-20230126152724-0fa3db229ce2 // indirect
)

tool (
	github.com/amnezia-vpn/amneziawg-go/v3
	golang.org/x/vuln/cmd/govulncheck
)
