```
go get -v github.com/buger/jsonparser
go get -v code.cloudfoundry.org/cli/plugin

cf uninstall-plugin ServiceManagement
GOOS=darwin GOARCH=amd64 go build -o ServiceManagement.osx ServiceManagement_plugin.go
chmod 755 ServiceManagement.osx

cf install-plugin ServiceManagement.osx -f

cf plugins | grep ServiceManage

```
