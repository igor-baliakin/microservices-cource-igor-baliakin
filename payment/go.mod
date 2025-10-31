module github.com/igor-baliakin/microservices-cource-igor-baliakin/payment

go 1.25.2

replace github.com/igor-baliakin/microservices-cource-igor-baliakin/shared => ../shared

require (
	github.com/google/uuid v1.6.0
	github.com/igor-baliakin/microservices-cource-igor-baliakin/shared v0.0.0-00010101000000-000000000000
	google.golang.org/grpc v1.76.0
)

require (
	golang.org/x/net v0.42.0 // indirect
	golang.org/x/sys v0.34.0 // indirect
	golang.org/x/text v0.27.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250804133106-a7a43d27e69b // indirect
	google.golang.org/protobuf v1.36.10 // indirect
)
