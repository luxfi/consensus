module github.com/luxfi/consensus/examples/01-simple-bridge

go 1.25.5

require github.com/luxfi/dex v0.0.0

require (
	github.com/gorilla/websocket v1.5.4-0.20250319132907-e064f32e3674 // indirect
	github.com/shopspring/decimal v1.4.0 // indirect
	golang.org/x/net v0.48.0 // indirect
)

replace github.com/luxfi/dex => ../../../dex
