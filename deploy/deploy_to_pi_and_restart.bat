set GOOS=linux
set GOARM=5
set GOARCH=arm
go build -tags static
scp music alex@raspberrypi:
type deploy\deploy_and_restart.sh | ssh alex@raspberrypi
