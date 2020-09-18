```
go get -v github.com/buger/jsonparser
go get -v code.cloudfoundry.org/cli/plugin

cf uninstall-plugin ServiceManagement

GOOS=darwin GOARCH=amd64 go build -o ServiceManagement.osx ServiceManagement_plugin.go
chmod 755 ServiceManagement.osx
cf uninstall-plugin ServiceManagement ; cf install-plugin ServiceManagement.osx -f
cf plugins | grep ServiceManage


GOOS=darwin GOARCH=amd64 go build -o ServiceManagement.osx ServiceManagement_plugin.go ; chmod 755 ServiceManagement.osx ; cf uninstall-plugin ServiceManagement ; cf install-plugin ServiceManagement.osx -f ; cf plugins | grep ServiceManage


```

In BAS
```
cd ~
curl -o ServiceManagement_1_0_2.linux64 http://thedrop.sap-partner-eng.com/files/ServiceManagement_1_0_2.linux64
chmod 755 ServiceManagement_1_0_2.linux64
cf uninstall-plugin ServiceManagement ; cf install-plugin ServiceManagement_1_0_2.linux64 -f
cf plugins | grep ServiceManage

curl -LJO "Redirects"

curl -o get_smsi http://thedrop.sap-partner-eng.com/files/get_smsi
chmod 755 get_smsi
./get_smsi
```

Andrew Testing
```
cd projects
git clone git@github.com:SAP-samples/cloud-cap-multitenancy.git
git clone https://github.com/SAP-samples/cloud-cap-multitenancy.git
cd ~
ssh-keygen
cat ~/.ssh/id_rsa.pub
<Import into github SSH keys>
cf api https://api.cf.us10.hana.ondemand.com
cf login -u andrew.lunde@sap.com
3<. ae67provider>
cf smsi CAPMT_SMC -o SQLTools > smc.json

jq '.["sqltools.connections"]' smc.json

jq '.["sqltools.connections"] = "[]"' /home/user/.theia/settings.json

vim /home/user/.theia/settings.json smc.json

```